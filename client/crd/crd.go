package crd

import (
	"fmt"
	"time"

	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiextensionscli "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/spotahome/kooper/log"
	wraptime "github.com/spotahome/kooper/wrapper/time"
)

const (
	checkCRDInterval = 500 * time.Millisecond
	crdReadyTimeout  = 30 * time.Second
)

// Scope is the scope of a CRD.
type Scope = apiextensionsv1beta1.ResourceScope

const (
	// ClusterScoped represents a type of a cluster scoped CRD.
	ClusterScoped = apiextensionsv1beta1.ClusterScoped
	// NamespaceScoped represents a type of a namespaced scoped CRD.
	NamespaceScoped = apiextensionsv1beta1.NamespaceScoped
)

// Conf is the configuration required to create a CRD
type Conf struct {
	Kind       string
	NamePlural string
	Group      string
	Version    string
	Scope      Scope
}

func (c *Conf) getName() string {
	return fmt.Sprintf("%s.%s", c.NamePlural, c.Group)
}

// Interface is the CRD client that knows how to interact with k8s to manage them.
type Interface interface {
	// EnsureCreated will ensure the the CRD is present.
	EnsurePresent(conf Conf) error
	// WaitToBePresent will wait until the CRD is present or it timesout.
	WaitToBePresent(name string, timeout time.Duration) error
	// Delete will delete the CRD.
	Delete(name string) error
}

// Client is the CRD client implementation using API calls to kubernetes.
type Client struct {
	aeClient apiextensionscli.Interface
	logger   log.Logger
	time     wraptime.Time // Use a time wrapper so we can control the time on our tests.
}

// NewClient returns a new CRD client.
func NewClient(aeClient apiextensionscli.Interface, logger log.Logger) *Client {
	return NewCustomClient(aeClient, wraptime.Base, logger)
}

// NewCustomClient returns a new CRD client letting you set all the required parameters
func NewCustomClient(aeClient apiextensionscli.Interface, time wraptime.Time, logger log.Logger) *Client {
	return &Client{
		aeClient: aeClient,
		logger:   logger,
		time:     time,
	}
}

// EnsurePresent satisfies crd.Interface.
func (c *Client) EnsurePresent(conf Conf) error {
	// TODO: Check version of cluster equal or greater than 1.7
	crdName := conf.getName()

	crd := &apiextensionsv1beta1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: crdName,
		},
		Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
			Group:   conf.Group,
			Version: conf.Version,
			Scope:   conf.Scope,
			Names: apiextensionsv1beta1.CustomResourceDefinitionNames{
				Plural: conf.NamePlural,
				Kind:   conf.Kind,
			},
		},
	}

	_, err := c.aeClient.ApiextensionsV1beta1().CustomResourceDefinitions().Create(crd)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("error creating crd %s: %s", crdName, err)
		}
		return nil
	}

	c.logger.Infof("crd %s created, waitint to be ready...", crdName)
	c.WaitToBePresent(crdName, crdReadyTimeout)
	c.logger.Infof("crd %s ready", crdName)

	return nil
}

// WaitToBePresent satisfies crd.Interface.
func (c *Client) WaitToBePresent(name string, timeout time.Duration) error {
	tout := c.time.After(timeout)
	t := c.time.NewTicker(checkCRDInterval)

	for {
		select {
		case <-t.C:
			_, err := c.aeClient.ApiextensionsV1beta1().CustomResourceDefinitions().Get(name, metav1.GetOptions{})
			// Is present, finish.
			if err == nil {
				return nil
			}
		case <-tout:
			return fmt.Errorf("timeout waiting for CRD")
		}
	}
}

// Delete satisfies crd.Interface.
func (c *Client) Delete(name string) error {
	return c.aeClient.ApiextensionsV1beta1().CustomResourceDefinitions().Delete(name, &metav1.DeleteOptions{})
}
