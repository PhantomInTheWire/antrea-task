package main

import (
	"context"
	"flag"
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
	node := requireNodeName()
	cfg := loadKubeConfig()
	cs := newClientset(cfg)
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	if err := NewController(cs, node).Run(ctx); err != nil {
		klog.Fatal(err)
	}
}

func requireNodeName() string {
	node := os.Getenv("NODE_NAME")
	if node == "" {
		klog.Fatal("NODE_NAME required")
	}
	return node
}

func loadKubeConfig() *rest.Config {
	cfg, err := rest.InClusterConfig()
	if err == nil {
		return cfg
	}
	cfg, err = clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
	if err != nil {
		klog.Fatal(err)
	}
	return cfg
}

func newClientset(cfg *rest.Config) kubernetes.Interface {
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		klog.Fatal(err)
	}
	return cs
}
