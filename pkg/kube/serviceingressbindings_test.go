package kube

import (
	"reflect"
	"testing"

	"github.com/skupperproject/skupper/api/types"
	"github.com/skupperproject/skupper/pkg/service"
	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

type TestContext struct {
	client    kubernetes.Interface
	namespace string
}

func (s *TestContext) GetService(name string) (*corev1.Service, bool, error) {
	svc, err := s.client.CoreV1().Services(s.namespace).Get(name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return nil, false, nil
	} else if err != nil {
		return nil, false, err
	}
	return svc, true, nil
}

func (s *TestContext) DeleteService(svc *corev1.Service) error {
	return s.client.CoreV1().Services(s.namespace).Delete(svc.ObjectMeta.Name, &metav1.DeleteOptions{})
}

func (s *TestContext) CreateService(svc *corev1.Service) error {
	_, err := s.client.CoreV1().Services(s.namespace).Create(svc)
	return err
}

func (s *TestContext) UpdateService(svc *corev1.Service) error {
	_, err := s.client.CoreV1().Services(s.namespace).Update(svc)
	return err
}

func (s *TestContext) IsOwned(service *corev1.Service) bool {
	if controlled, ok := service.ObjectMeta.Annotations[types.ControlledQualifier]; ok {
		return controlled == "true"
	}
	return false
}

func (s *TestContext) AllServices() (map[string]corev1.Service, error) {
	list, err := s.client.CoreV1().Services(s.namespace).List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	svcs := map[string]corev1.Service{}
	for _, svc := range list.Items {
		svcs[svc.ObjectMeta.Name] = svc
	}
	return svcs, nil
}

func (c *TestContext) NewTargetResolver(address string, selector string) (service.TargetResolver, error) {
	return nil, nil
}

func (c *TestContext) NewServiceIngress(def *types.ServiceInterface) service.ServiceIngress {
	if def.Headless != nil {
		return NewHeadlessServiceIngress(c, def.Origin)
	}
	return NewServiceIngressAlways(c)
}

func TestServiceIngressBindings(t *testing.T) {
	context := &TestContext{
		client:    fake.NewSimpleClientset(),
		namespace: "test",
	}
	type scenario struct {
		name           string
		definition     types.ServiceInterface
		allocatedPorts []int
		existing       []corev1.Service
		expected       []corev1.Service
	}
	scenarios := []scenario{
		{
			name: "simple create",
			definition: types.ServiceInterface{
				Address:  "foo",
				Protocol: "tcp",
				Ports:    []int{8080},
			},
			allocatedPorts: []int{1024},
			existing:       []corev1.Service{},
			expected: []corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo",
					},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{
							"application":          "skupper-router",
							"skupper.io/component": "router",
						},
						Ports: []corev1.ServicePort{
							{
								Port:       8080,
								TargetPort: intstr.FromInt(1024),
								Protocol:   "TCP",
							},
						},
					},
				},
			},
		},
		{
			name: "simple update",
			definition: types.ServiceInterface{
				Address:  "foo",
				Protocol: "tcp",
				Ports:    []int{8080},
			},
			allocatedPorts: []int{1024},
			existing: []corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo",
					},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{
							"foo": "bar",
						},
						Ports: []corev1.ServicePort{
							{
								Port:       8080,
								TargetPort: intstr.FromInt(9090),
								Protocol:   "TCP",
							},
						},
					},
				},
			},
			expected: []corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo",
					},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{
							"application":          "skupper-router",
							"skupper.io/component": "router",
						},
						Ports: []corev1.ServicePort{
							{
								Port:       8080,
								TargetPort: intstr.FromInt(1024),
								Protocol:   "TCP",
							},
						},
					},
				},
			},
		},
		{
			name: "simple no change needed",
			definition: types.ServiceInterface{
				Address:  "foo",
				Protocol: "tcp",
				Ports:    []int{8080},
			},
			allocatedPorts: []int{1024},
			existing: []corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo",
					},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{
							"application":          "skupper-router",
							"skupper.io/component": "router",
						},
						Ports: []corev1.ServicePort{
							{
								Port:       8080,
								TargetPort: intstr.FromInt(1024),
								Protocol:   "TCP",
							},
						},
					},
				},
			},
			expected: []corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo",
					},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{
							"application":          "skupper-router",
							"skupper.io/component": "router",
						},
						Ports: []corev1.ServicePort{
							{
								Port:       8080,
								TargetPort: intstr.FromInt(1024),
								Protocol:   "TCP",
							},
						},
					},
				},
			},
		},
		{
			name: "headless local",
			definition: types.ServiceInterface{
				Address:  "foo",
				Protocol: "tcp",
				Ports:    []int{8080},
				Headless: &types.Headless{
					Name: "foo",
					Size: 1,
				},
			},
			allocatedPorts: []int{1024},
			existing:       []corev1.Service{},
			expected:       []corev1.Service{},
		},
		{
			name: "headless remote",
			definition: types.ServiceInterface{
				Address:  "foo",
				Protocol: "tcp",
				Ports:    []int{8080},
				Headless: &types.Headless{
					Name: "foo",
					Size: 1,
				},
				Origin: "xyz",
			},
			allocatedPorts: []int{1024},
			existing:       []corev1.Service{},
			expected: []corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo",
					},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{
							"internal.skupper.io/service": "foo",
						},
						Ports: []corev1.ServicePort{
							{
								Port:       8080,
								TargetPort: intstr.FromInt(1024),
								Protocol:   "TCP",
							},
						},
					},
				},
			},
		},
		{
			name: "headless remote update",
			definition: types.ServiceInterface{
				Address:  "foo",
				Protocol: "tcp",
				Ports:    []int{8080},
				Headless: &types.Headless{
					Name: "foo",
					Size: 1,
				},
				Origin: "xyz",
				Labels: map[string]string{
					"foo": "bar",
				},
			},
			allocatedPorts: []int{1024},
			existing: []corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo",
					},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{
							"internal.skupper.io/service": "foo",
						},
						Ports: []corev1.ServicePort{
							{
								Port:       8080,
								TargetPort: intstr.FromInt(9090),
								Protocol:   "TCP",
							},
						},
					},
				},
			},
			expected: []corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo",
						Labels: map[string]string{
							"foo": "bar",
						},
					},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{
							"internal.skupper.io/service": "foo",
						},
						Ports: []corev1.ServicePort{
							{
								Port:       8080,
								TargetPort: intstr.FromInt(1024),
								Protocol:   "TCP",
							},
						},
					},
				},
			},
		},
		{
			name: "headless remote no change needed",
			definition: types.ServiceInterface{
				Address:  "foo",
				Protocol: "tcp",
				Ports:    []int{8080},
				Headless: &types.Headless{
					Name: "foo",
					Size: 1,
				},
				Origin: "xyz",
				Labels: map[string]string{
					"foo": "bar",
				},
			},
			allocatedPorts: []int{1024},
			existing: []corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo",
						Labels: map[string]string{
							"foo": "bar",
						},
					},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{
							"internal.skupper.io/service": "foo",
						},
						Ports: []corev1.ServicePort{
							{
								Port:       8080,
								TargetPort: intstr.FromInt(1024),
								Protocol:   "TCP",
							},
						},
					},
				},
			},
			expected: []corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo",
						Labels: map[string]string{
							"foo": "bar",
						},
					},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{
							"internal.skupper.io/service": "foo",
						},
						Ports: []corev1.ServicePort{
							{
								Port:       8080,
								TargetPort: intstr.FromInt(1024),
								Protocol:   "TCP",
							},
						},
					},
				},
			},
		},
	}
	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			for _, svc := range s.existing {
				err := context.CreateService(&svc)
				assert.Assert(t, err == nil)
			}
			bindings := service.NewServiceBindings(s.definition, s.allocatedPorts, context)
			err := bindings.RealiseIngress()
			assert.Assert(t, err == nil)
			actual, err := context.AllServices()
			assert.Equal(t, len(actual), len(s.expected))
			var cleanup []corev1.Service
			for _, expectedSvc := range s.expected {
				actualSvc, ok := actual[expectedSvc.ObjectMeta.Name]
				assert.Assert(t, ok)
				assert.Equal(t, actualSvc.ObjectMeta.Name, expectedSvc.ObjectMeta.Name)
				assert.Assert(t, reflect.DeepEqual(actualSvc.Spec.Selector, expectedSvc.Spec.Selector), "expected %v, got %v", expectedSvc.Spec.Selector, actualSvc.Spec.Selector)
				assert.Assert(t, reflect.DeepEqual(IndexServicePorts(&actualSvc), IndexServicePorts(&expectedSvc)), "expected %v, got %v", IndexServicePorts(&expectedSvc), IndexServicePorts(&actualSvc))
				assert.Assert(t, reflect.DeepEqual(actualSvc.ObjectMeta.Labels, expectedSvc.ObjectMeta.Labels), "expected %v, got %v", expectedSvc.ObjectMeta.Labels, actualSvc.ObjectMeta.Labels)
				delete(actual, expectedSvc.ObjectMeta.Name)
				cleanup = append(cleanup, actualSvc)
			}
			assert.Equal(t, len(actual), 0)

			//cleanup
			for _, svc := range cleanup {
				err := context.DeleteService(&svc)
				assert.Assert(t, err == nil)
			}
		})
	}
}

func TestGetApplicationSelector(t *testing.T) {
	type scenario struct {
		name     string
		service  *corev1.Service
		expected string
	}
	scenarios := []scenario{
		{
			name: "simple",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{
						"foo":                  "bar",
						"application":          "skupper-router",
						"skupper.io/component": "router",
					},
				},
			},
			expected: "foo=bar",
		},
	}
	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			result := GetApplicationSelector(s.service)
			assert.Equal(t, result, s.expected)
		})
	}
}

func TestServiceIngressMatches(t *testing.T) {
	type scenario struct {
		name     string
		first    types.ServiceInterface
		second   types.ServiceInterface
		expected bool
	}
	scenarios := []scenario{
		{
			name:  "changed to headless",
			first: types.ServiceInterface{},
			second: types.ServiceInterface{
				Headless: &types.Headless{},
			},
			expected: false,
		},
		{
			name:     "stayed normal",
			first:    types.ServiceInterface{},
			second:   types.ServiceInterface{},
			expected: true,
		},
		{
			name: "stayed headless local",
			first: types.ServiceInterface{
				Headless: &types.Headless{},
			},
			second: types.ServiceInterface{
				Headless: &types.Headless{},
			},
			expected: true,
		},
		{
			name: "stayed headless remote",
			first: types.ServiceInterface{
				Headless: &types.Headless{},
				Origin:   "abc",
			},
			second: types.ServiceInterface{
				Headless: &types.Headless{},
				Origin:   "abc",
			},
			expected: true,
		},
		{
			name: "changed from headless to normal",
			first: types.ServiceInterface{
				Headless: &types.Headless{},
			},
			second:   types.ServiceInterface{},
			expected: false,
		},
		{
			name: "changed from headless remote to local",
			first: types.ServiceInterface{
				Headless: &types.Headless{},
				Origin:   "abc",
			},
			second: types.ServiceInterface{
				Headless: &types.Headless{},
			},
			expected: false,
		},
		{
			name: "changed from headless local to remote",
			first: types.ServiceInterface{
				Headless: &types.Headless{},
			},
			second: types.ServiceInterface{
				Headless: &types.Headless{},
				Origin:   "abc",
			},
			expected: false,
		},
	}
	context := &TestContext{}
	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			ingress := context.NewServiceIngress(&s.first)
			result := ingress.Matches(&s.second)
			assert.Equal(t, result, s.expected)
		})
	}
}
