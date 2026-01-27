package k8s

const DefaultApplicationname = "talaria"

type K8sOptions struct {
	// Namespace to watch for services
	Namespace string `json:"namespace" yaml:"namespace"`

	// LabelSelector used to filter services/pods, e.g. "app=my-service"
	LabelSelector string `json:"labelSelector" yaml:"labelSelector"`

	// The name of the service/endpoints to discover
	ServiceName string `json:"serviceName" yaml:"serviceName"`

	// Optional: In-cluster vs out-of-cluster config, kubeconfig path, etc.
	InCluster    bool   `json:"inCluster" yaml:"inCluster"`
	Kubeconfig   string `json:"kubeconfig" yaml:"kubeconfig"`
	PortName     string `json:"portName" yaml:"portName"`
	Scheme       string `json:"scheme" yaml:"scheme"`
	EndpointType string `json:"endpointType" yaml:"endpointType"` // "pods" or "endpoints"
}
