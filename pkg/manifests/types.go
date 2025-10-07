package manifests

// ObjectMeta represents standard Kubernetes object metadata.
type ObjectMeta struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// TypeMeta represents the type information for Kubernetes resources.
type TypeMeta struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
}

// Namespace represents a Kubernetes namespace.
type Namespace struct {
	TypeMeta   `json:",inline"`
	ObjectMeta `json:"metadata"`
}

// CatalogSource represents an OLM CatalogSource.
type CatalogSource struct {
	TypeMeta   `json:",inline"`
	ObjectMeta `json:"metadata"`
	Spec       CatalogSourceSpec `json:"spec"`
}

// CatalogSourceSpec defines the spec for a CatalogSource.
type CatalogSourceSpec struct {
	SourceType  string `json:"sourceType"`
	Image       string `json:"image"`
	DisplayName string `json:"displayName"`
	Publisher   string `json:"publisher"`
}

// OperatorGroup represents an OLM OperatorGroup.
type OperatorGroup struct {
	TypeMeta   `json:",inline"`
	ObjectMeta `json:"metadata"`
	Spec       OperatorGroupSpec `json:"spec"`
}

// OperatorGroupSpec defines the spec for an OperatorGroup.
type OperatorGroupSpec struct {
	// Empty for now, can be extended as needed.
}

// Subscription represents an OLM Subscription.
type Subscription struct {
	TypeMeta   `json:",inline"`
	ObjectMeta `json:"metadata"`
	Spec       SubscriptionSpec `json:"spec"`
}

// SubscriptionSpec defines the spec for a Subscription.
type SubscriptionSpec struct {
	Channel             string `json:"channel"`
	Name                string `json:"name"`
	Source              string `json:"source"`
	SourceNamespace     string `json:"sourceNamespace"`
	InstallPlanApproval string `json:"installPlanApproval"`
}

// ImageDigestMirrorSet represents an OpenShift ImageDigestMirrorSet.
type ImageDigestMirrorSet struct {
	TypeMeta   `json:",inline"`
	ObjectMeta `json:"metadata"`
	Spec       ImageDigestMirrorSetSpec `json:"spec"`
}

// ImageDigestMirrorSetSpec defines the spec for an ImageDigestMirrorSet.
type ImageDigestMirrorSetSpec struct {
	ImageDigestMirrors []ImageDigestMirror `json:"imageDigestMirrors"`
}

// ImageDigestMirror defines a source-to-mirrors mapping.
type ImageDigestMirror struct {
	Source  string   `json:"source"`
	Mirrors []string `json:"mirrors"`
}

// CatalogMetadata contains metadata about a catalog image.
type CatalogMetadata struct {
	Image       string // Full image reference
	Digest      string // Full digest
	ShortDigest string // Short digest for naming
	CatalogType string // Type of catalog (e.g., catalog-ystream)
}

// ManifestSet contains all manifests needed for deployment.
type ManifestSet struct {
	Namespace     *Namespace
	IDMS          *ImageDigestMirrorSet
	CatalogSource *CatalogSource
	OperatorGroup *OperatorGroup
	Subscription  *Subscription
}
