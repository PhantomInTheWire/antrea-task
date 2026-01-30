package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

const (
	annotationKey = "tcpdump.antrea.io"
)

type captureProcess struct {
	cmd     *exec.Cmd
	stopCh  chan struct{}
	stopped bool
	mu      sync.Mutex
}

type Controller struct {
	clientset kubernetes.Interface
	nodeName  string
	captures  map[string]*captureProcess
	mu        sync.RWMutex
	stopCh    chan struct{}
}

type stderrBuffer struct {
	podKey string
}

func (s *stderrBuffer) Write(p []byte) (n int, err error) {
	klog.Errorf("tcpdump stderr for pod %s: %s", s.podKey, string(p))
	return len(p), nil
}

func NewController(clientset kubernetes.Interface, nodeName string) *Controller {
	return &Controller{
		clientset: clientset,
		nodeName:  nodeName,
		captures:  make(map[string]*captureProcess),
		stopCh:    make(chan struct{}),
	}
}

func (c *Controller) Run(ctx context.Context) error {
	klog.Infof("Starting controller for node: %s", c.nodeName)

	fieldSelector := fields.OneTermEqualSelector("spec.nodeName", c.nodeName).String()
	factory := informers.NewSharedInformerFactoryWithOptions(
		c.clientset,
		time.Minute,
		informers.WithTweakListOptions(func(options *metav1.ListOptions) {
			options.FieldSelector = fieldSelector
		}),
	)

	podInformer := factory.Core().V1().Pods().Informer()

	if _, err := podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.handlePodAdd,
		UpdateFunc: c.handlePodUpdate,
		DeleteFunc: c.handlePodDelete,
	}); err != nil {
		return fmt.Errorf("failed to add event handler: %w", err)
	}

	factory.Start(c.stopCh)

	if !cache.WaitForCacheSync(ctx.Done(), podInformer.HasSynced) {
		return fmt.Errorf("failed to sync caches")
	}

	klog.Info("Controller synced and ready")

	<-ctx.Done()
	return c.Shutdown()
}

func (c *Controller) Shutdown() error {
	klog.Info("Shutting down controller, stopping all captures")
	close(c.stopCh)

	c.mu.Lock()
	defer c.mu.Unlock()

	for podName, cap := range c.captures {
		klog.Infof("Stopping capture for pod: %s", podName)
		c.stopCaptureInternal(podName, cap)
	}

	return nil
}

func (c *Controller) handlePodAdd(obj any) {
	pod := obj.(*corev1.Pod)
	klog.V(4).Infof("Pod added: %s/%s", pod.Namespace, pod.Name)
	c.processPod(pod)
}

func (c *Controller) handlePodUpdate(oldObj, newObj any) {
	oldPod := oldObj.(*corev1.Pod)
	newPod := newObj.(*corev1.Pod)

	oldAnnotation := oldPod.Annotations[annotationKey]
	newAnnotation := newPod.Annotations[annotationKey]

	if oldAnnotation != newAnnotation {
		klog.Infof("Pod %s/%s annotation changed from '%s' to '%s'",
			newPod.Namespace, newPod.Name, oldAnnotation, newAnnotation)
		c.processPod(newPod)
	}
}

func (c *Controller) handlePodDelete(obj any) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			klog.Errorf("Couldn't get object from tombstone %#v", obj)
			return
		}
		pod, ok = tombstone.Obj.(*corev1.Pod)
		if !ok {
			klog.Errorf("Tombstone contained object that is not a Pod %#v", obj)
			return
		}
	}
	klog.Infof("Pod deleted: %s/%s", pod.Namespace, pod.Name)
	c.stopCapture(c.podKey(pod))
}

func (c *Controller) podKey(pod *corev1.Pod) string {
	return pod.Namespace + "/" + pod.Name
}

func (c *Controller) processPod(pod *corev1.Pod) {
	podKey := c.podKey(pod)
	annotation := pod.Annotations[annotationKey]

	if annotation == "" {
		c.stopCapture(podKey)
		return
	}

	maxFiles, err := strconv.Atoi(annotation)
	if err != nil || maxFiles <= 0 {
		klog.Errorf("Invalid annotation value '%s' for pod %s: %v", annotation, podKey, err)
		c.stopCapture(podKey)
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.captures[podKey]; exists {
		klog.V(4).Infof("Capture already running for pod %s, skipping", podKey)
		return
	}

	c.startCaptureInternal(podKey, pod.Name, maxFiles)
}

func (c *Controller) startCaptureInternal(podKey string, podName string, maxFiles int) {
	if _, exists := c.captures[podKey]; exists {
		return
	}

	captureFile := fmt.Sprintf("/capture-%s.pcap", podName)
	klog.Infof("Starting capture for pod %s (max files: %d, output: %s)",
		podKey, maxFiles, captureFile)

	cmd := exec.Command("tcpdump",
		"-C", "1",
		"-W", strconv.Itoa(maxFiles),
		"-w", captureFile,
		"-i", "any",
		"-n",
		"-Z", "root",
	)

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	cmd.Stderr = &stderrBuffer{podKey: podKey}

	if err := cmd.Start(); err != nil {
		klog.Errorf("Failed to start tcpdump for pod %s: %v", podKey, err)
		return
	}

	cap := &captureProcess{
		cmd:     cmd,
		stopCh:  make(chan struct{}),
		stopped: false,
	}
	c.captures[podKey] = cap

	go func() {
		if err := cmd.Wait(); err != nil {
			klog.V(4).Infof("tcpdump for pod %s exited: %v", podKey, err)
		}

		c.mu.Lock()
		cap.mu.Lock()
		if !cap.stopped {
			delete(c.captures, podKey)
			cap.stopped = true
			close(cap.stopCh)
		}
		cap.mu.Unlock()
		c.mu.Unlock()
	}()
}

func (c *Controller) stopCapture(podName string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	cap, exists := c.captures[podName]
	if !exists {
		return
	}

	c.stopCaptureInternal(podName, cap)
}

func (c *Controller) stopCaptureInternal(podName string, cap *captureProcess) {
	klog.Infof("Stopping capture for pod %s", podName)

	cap.mu.Lock()
	if cap.stopped {
		cap.mu.Unlock()
		return
	}
	cap.stopped = true
	close(cap.stopCh)
	cap.mu.Unlock()

	if cap.cmd.Process != nil {
		klog.V(4).Infof("Sending SIGTERM to tcpdump process for pod %s", podName)
		if err := syscall.Kill(-cap.cmd.Process.Pid, syscall.SIGTERM); err != nil {
			klog.V(4).Infof("Failed to send SIGTERM to tcpdump process for pod %s: %v", podName, err)
		}

		done := make(chan error, 1)
		go func() {
			done <- cap.cmd.Wait()
		}()

		select {
		case <-done:
			klog.V(4).Infof("tcpdump for pod %s terminated gracefully", podName)
		case <-time.After(5 * time.Second):
			klog.Warningf("tcpdump for pod %s did not terminate gracefully, killing", podName)
			if err := syscall.Kill(-cap.cmd.Process.Pid, syscall.SIGKILL); err != nil {
				klog.V(4).Infof("Failed to send SIGKILL to tcpdump process for pod %s: %v", podName, err)
			}
			<-done
		}
	}

	delete(c.captures, podName)

	c.cleanupFiles(podName)
}

func (c *Controller) cleanupFiles(podKey string) {
	parts := strings.Split(podKey, "/")
	podName := parts[len(parts)-1]

	pattern := fmt.Sprintf("/capture-%s.pcap*", podName)
	klog.Infof("Cleaning up capture files matching: %s", pattern)

	matches, err := filepath.Glob(pattern)
	if err != nil {
		klog.Errorf("Failed to glob pattern %s: %v", pattern, err)
		return
	}

	failedDeletions := 0
	for _, file := range matches {
		klog.V(4).Infof("Deleting capture file: %s", file)
		if err := os.Remove(file); err != nil {
			klog.Errorf("Failed to delete file %s: %v", file, err)
			failedDeletions++
		}
	}

	if len(matches) == 0 {
		klog.V(4).Infof("No capture files found for pod %s", podKey)
	} else {
		if failedDeletions > 0 {
			klog.Warningf("Deleted %d of %d capture file(s) for pod %s (%d failed)",
				len(matches)-failedDeletions, len(matches), podKey, failedDeletions)
		} else {
			klog.Infof("Deleted %d capture file(s) for pod %s", len(matches), podKey)
		}
	}
}
