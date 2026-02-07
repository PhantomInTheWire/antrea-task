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
	cmd *exec.Cmd
}

type Controller struct {
	clientset kubernetes.Interface
	nodeName  string
	captures  map[string]*captureProcess
	mu        sync.RWMutex
	stopCh    chan struct{}
}

func NewController(cs kubernetes.Interface, node string) *Controller {
	return &Controller{clientset: cs, nodeName: node, captures: make(map[string]*captureProcess), stopCh: make(chan struct{})}
}

func (c *Controller) Run(ctx context.Context) error {
	fieldSelector := fields.OneTermEqualSelector("spec.nodeName", c.nodeName).String()
	factory := informers.NewSharedInformerFactoryWithOptions(c.clientset, time.Minute,
		informers.WithTweakListOptions(func(o *metav1.ListOptions) { o.FieldSelector = fieldSelector }))
	podInformer := c.newPodInformer(factory)
	factory.Start(c.stopCh)
	if !cache.WaitForCacheSync(ctx.Done(), podInformer.HasSynced) {
		return fmt.Errorf("failed to sync caches")
	}
	<-ctx.Done()
	return c.Shutdown()
}

func (c *Controller) Shutdown() error {
	close(c.stopCh)
	c.mu.Lock()
	defer c.mu.Unlock()
	for podKey, capture := range c.captures {
		c.stopCaptureProcess(podKey, capture)
	}
	return nil
}

func (c *Controller) newPodInformer(factory informers.SharedInformerFactory) cache.SharedIndexInformer {
	podInformer := factory.Core().V1().Pods().Informer()
	podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(o any) { c.onPodAddOrUpdate(o.(*corev1.Pod)) },
		UpdateFunc: func(_, n any) { c.onPodAddOrUpdate(n.(*corev1.Pod)) },
		DeleteFunc: c.onPodDelete,
	})
	return podInformer
}

func (c *Controller) onPodAddOrUpdate(pod *corev1.Pod) {
	podKey := pod.Namespace + "/" + pod.Name
	annotation := pod.Annotations[annotationKey]
	if annotation == "" {
		c.stopCaptureForPodKey(podKey)
		return
	}
	maxFiles, err := strconv.Atoi(annotation)
	if err != nil || maxFiles <= 0 {
		klog.Errorf("Invalid annotation value '%s' for pod %s: %v", annotation, podKey, err)
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.captures[podKey]; exists {
		return
	}
	c.startCapture(podKey, pod, maxFiles)
}

func (c *Controller) onPodDelete(o any) {
	pod, ok := o.(*corev1.Pod)
	if !ok {
		pod = o.(cache.DeletedFinalStateUnknown).Obj.(*corev1.Pod)
	}
	c.stopCaptureForPodKey(pod.Namespace + "/" + pod.Name)
}

func (c *Controller) startCapture(podKey string, pod *corev1.Pod, maxFiles int) {
	podIP := pod.Status.PodIP
	if podIP == "" {
		klog.Errorf("Pod %s has no IP", podKey)
		return
	}
	captureFile := fmt.Sprintf("/capture-%s.pcap", pod.Name)
	cmd := exec.Command("tcpdump", "-i", "any", "-U", "-C", "1", "-W", strconv.Itoa(maxFiles), "-w", captureFile, "-Z", "root", "host", podIP)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		klog.Errorf("Failed to start tcpdump for pod %s: %v", podKey, err)
		return
	}
	klog.Infof("Started capture for pod %s (IP %s)", podKey, podIP)
	c.captures[podKey] = &captureProcess{cmd: cmd}
	go func() {
		_ = cmd.Wait()
		c.mu.Lock()
		delete(c.captures, podKey)
		c.mu.Unlock()
	}()
}

func (c *Controller) stopCaptureForPodKey(podKey string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if capture, exists := c.captures[podKey]; exists {
		c.stopCaptureProcess(podKey, capture)
	}
}

func (c *Controller) stopCaptureProcess(podKey string, capture *captureProcess) {
	terminateProcessGroup(capture.cmd)
	delete(c.captures, podKey)
	podName := podKey[strings.LastIndex(podKey, "/")+1:]
	matches, _ := filepath.Glob(fmt.Sprintf("/capture-%s.pcap*", podName))
	for _, file := range matches {
		_ = os.Remove(file)
	}
}

func terminateProcessGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-done
	}
}
