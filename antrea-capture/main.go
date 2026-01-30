package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

func main() {
	klog.InitFlags(nil)
	flag.Parse()

	nodeName := os.Getenv("NODE_NAME")
	if nodeName == "" {
		klog.Fatal("NODE_NAME environment variable is required")
	}

	klog.Infof("Starting packet capture controller on node: %s", nodeName)

	config, err := buildConfig()
	if err != nil {
		klog.Fatalf("Failed to build k8s config: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Fatalf("Failed to create k8s client: %v", err)
	}

	ctrl := NewController(clientset, nodeName)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		klog.Infof("Received signal: %v", sig)
		cancel()
	}()

	if err := ctrl.Run(ctx); err != nil {
		klog.Fatalf("Controller error: %v", err)
	}

	klog.Info("Controller stopped gracefully")
}

func buildConfig() (*rest.Config, error) {
	if _, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount/token"); err == nil {
		klog.Info("Running in-cluster, using service account")
		return rest.InClusterConfig()
	}

	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		kubeconfig = os.Getenv("HOME") + "/.kube/config"
	}

	if _, err := os.Stat(kubeconfig); err == nil {
		klog.Infof("Using kubeconfig: %s", kubeconfig)
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}

	return nil, fmt.Errorf("unable to find kubeconfig or in-cluster config")
}
