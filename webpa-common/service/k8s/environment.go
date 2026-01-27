package k8s

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/sd"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

// k8sInstancer implements sd.Instancer for Kubernetes.
type k8sInstancer struct {
	mtx       sync.RWMutex
	endpoints []string
	logger    log.Logger

	subscribers map[chan<- sd.Event]struct{}
}

func NewK8sInstancer(logger log.Logger, opts *K8sOptions) (sd.Instancer, error) {
	if opts == nil {
		return nil, fmt.Errorf("k8s options must not be nil")
	}
	if opts.ServiceName == "" {
		opts.ServiceName = DefaultApplicationname
	}

	cfg, err := buildK8sConfig(opts)
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("create k8s client: %w", err)
	}

	inst := &k8sInstancer{
		logger:      logger,
		subscribers: make(map[chan<- sd.Event]struct{}),
	}

	// Use shared informer to watch Endpoints or Pods
	factory := newInformerFactory(clientset, opts)
	informer := selectInformer(factory, opts)

	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { inst.updateFromStore(informer.GetStore(), opts) },
		UpdateFunc: func(oldObj, newObj interface{}) { inst.updateFromStore(informer.GetStore(), opts) },
		DeleteFunc: func(obj interface{}) { inst.updateFromStore(informer.GetStore(), opts) },
	})

	stopCh := make(chan struct{})

	go func() {
		defer runtime.HandleCrash()
		informer.Run(stopCh)
	}()

	// Wait for initial sync in a separate goroutine
	go func() {
		if !cache.WaitForCacheSync(stopCh, informer.HasSynced) {
			_ = logger.Log("level", "error", "msg", "k8s informer cache sync failed")
			return
		}
		inst.updateFromStore(informer.GetStore(), opts)
	}()

	return inst, nil
}

func buildK8sConfig(opts *K8sOptions) (*rest.Config, error) {
	if opts.InCluster {
		return rest.InClusterConfig()
	}
	if opts.Kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", opts.Kubeconfig)
	}
	// Fallback to default rules
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	cfgOverrides := &clientcmd.ConfigOverrides{}
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, cfgOverrides).ClientConfig()
}

// newInformerFactory builds a shared informer factory filtered to the namespace and label selector.
func newInformerFactory(clientset *kubernetes.Clientset, opts *K8sOptions) informers.SharedInformerFactory {
	tweak := func(lo *metav1.ListOptions) {
		if opts.LabelSelector != "" {
			lo.LabelSelector = opts.LabelSelector
		}
	}
	if opts.Namespace == "" {
		return informers.NewSharedInformerFactoryWithOptions(clientset, 30*time.Second,
			informers.WithTweakListOptions(tweak),
		)
	}
	return informers.NewSharedInformerFactoryWithOptions(clientset, 30*time.Second,
		informers.WithNamespace(opts.Namespace),
		informers.WithTweakListOptions(tweak),
	)
}

// selectInformer chooses between pod or endpoint-based discovery.
func selectInformer(factory informers.SharedInformerFactory, opts *K8sOptions) cache.SharedIndexInformer {
	switch opts.EndpointType {
	case "pods":
		return factory.Core().V1().Pods().Informer()
	default:
		// default to Endpoints
		return factory.Core().V1().Endpoints().Informer()
	}
}

func (i *k8sInstancer) updateFromStore(store cache.Store, opts *K8sOptions) {
	var eps []string

	for _, obj := range store.List() {
		switch o := obj.(type) {
		case *corev1.Endpoints:
			if o.Name != opts.ServiceName {
				continue
			}
			for _, subset := range o.Subsets {
				for _, addr := range subset.Addresses {
					for _, port := range subset.Ports {
						if opts.PortName != "" && port.Name != opts.PortName {
							continue
						}
						host := addr.IP
						if host == "" && addr.Hostname != "" {
							host = addr.Hostname
						}
						if host == "" {
							continue
						}
						scheme := opts.Scheme
						if scheme == "" {
							scheme = "http"
						}
						eps = append(eps, fmt.Sprintf("%s://%s:%d", scheme, net.JoinHostPort(host, fmt.Sprint(port.Port))))
					}
				}
			}
		case *corev1.Pod:
			if opts.EndpointType != "pods" {
				continue
			}
			// Pull pod IP and a known container port if desired
			if o.Status.PodIP == "" {
				continue
			}
			// Simplest case: one fixed port
			port := 80
			scheme := opts.Scheme
			if scheme == "" {
				scheme = "http"
			}
			eps = append(eps, fmt.Sprintf("%s://%s:%d", scheme, net.JoinHostPort(o.Status.PodIP, fmt.Sprint(port))))
		}
	}

	i.mtx.Lock()
	i.endpoints = eps
	for ch := range i.subscribers {
		ch <- sd.Event{Instances: eps, Err: nil}
	}
	i.mtx.Unlock()
}

// Implement sd.Instancer.

func (i *k8sInstancer) Register(ch chan<- sd.Event) {
	i.mtx.Lock()
	i.subscribers[ch] = struct{}{}
	// send initial state
	ch <- sd.Event{Instances: i.endpoints, Err: nil}
	i.mtx.Unlock()
}

func (i *k8sInstancer) Deregister(ch chan<- sd.Event) {
	i.mtx.Lock()
	delete(i.subscribers, ch)
	i.mtx.Unlock()
}

func (i *k8sInstancer) Stop() {
	// No-op if using shared factory; you can wire a stop channel if desired.
}
