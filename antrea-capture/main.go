package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	internalapi "k8s.io/cri-api/pkg/apis"
	criclient "k8s.io/cri-client/pkg"
	"k8s.io/klog/v2"
)

func main() {
	klog.InitFlags(nil)
	flag.Parse()
	node := requireNodeName()
	cs := mustNewClientset()
	runtimeSvc := mustNewRuntimeService()
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	if err := NewController(cs, node, runtimeSvc).Run(ctx); err != nil {
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

func mustNewClientset() kubernetes.Interface {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		cfg, err = clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
		if err != nil {
			klog.Fatal(err)
		}
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		klog.Fatal(err)
	}
	return cs
}

func mustNewRuntimeService() internalapi.RuntimeService {
	endpoint := runtimeEndpoint()
	runtimeSvc, err := criclient.NewRemoteRuntimeService(endpoint, 5*time.Second, nil, nil)
	if err != nil {
		klog.Fatalf("Failed to connect to CRI runtime at %s: %v", endpoint, err)
	}
	return runtimeSvc
}

func runtimeEndpoint() string {
	endpoint := os.Getenv("CRI_SOCKET")
	if endpoint == "" {
		klog.Fatal("CRI_SOCKET env var not set")
	}
	return endpoint
}
