package flow

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/jexia/semaphore/pkg/broker"
	"github.com/jexia/semaphore/pkg/broker/logger"
	"github.com/jexia/semaphore/pkg/codec/json"
	"github.com/jexia/semaphore/pkg/functions"
	"github.com/jexia/semaphore/pkg/references"
	"github.com/jexia/semaphore/pkg/specs"
	"github.com/jexia/semaphore/pkg/specs/labels"
	"github.com/jexia/semaphore/pkg/specs/template"
	"github.com/jexia/semaphore/pkg/specs/types"
	"github.com/jexia/semaphore/pkg/transport"
)

type transporter struct {
	err    error
	body   []byte
	status int
}

func (t *transporter) SendMsg(ctx context.Context, writer transport.ResponseWriter, request *transport.Request, refs references.Store) error {
	writer.HeaderStatus(t.status)

	go func() {
		writer.Write(t.body)
		writer.Close()
	}()

	return t.err
}

func (t *transporter) GetMethods() []transport.Method         { return nil }
func (t *transporter) GetMethod(name string) transport.Method { return nil }
func (t *transporter) Close() error                           { return nil }

func NewMockTransport(err error, status int, body []byte) transport.Call {
	if status == 0 {
		status = transport.StatusOK
	}

	return &transporter{
		err:    err,
		body:   body,
		status: status,
	}
}

type fncounter struct {
	counter int
	mutex   sync.Mutex
	err     error
}

func (counter *fncounter) handle(references.Store) error {
	counter.mutex.Lock()
	counter.counter++
	counter.mutex.Unlock()
	return counter.err
}

func TestNewRequest(t *testing.T) {
	request := NewRequest(nil, nil, nil)
	if request == nil {
		t.Fatal("unexpected result, expected a request to be returned")
	}
}

func TestNewCall(t *testing.T) {
	ctx := logger.WithLogger(broker.NewContext())
	node := &specs.Node{}
	options := &CallOptions{}

	result := NewCall(ctx, node, options)
	if result == nil {
		t.Fatal("unexpected result, expected a call to be constructed")
	}
}

func TestCallExecution(t *testing.T) {
	type test struct {
		node    *specs.Node
		options *CallOptions
		store   references.Store
	}

	constructor := json.NewConstructor()
	codec, _ := constructor.New("", &specs.ParameterMap{})

	tests := map[string]*test{
		"empty": {
			node: &specs.Node{},
			options: &CallOptions{
				Request:  &Request{},
				Response: &Request{},
			},
		},
		"request codec": {
			node: &specs.Node{},
			options: &CallOptions{
				Request: &Request{
					codec: codec,
				},
				Response: &Request{},
			},
		},
		"response codec": {
			node: &specs.Node{},
			options: &CallOptions{
				Request: &Request{},
				Response: &Request{
					codec: codec,
				},
			},
		},
		"request functions": {
			node: &specs.Node{},
			options: &CallOptions{
				Request: &Request{
					functions: functions.Stack{
						"sample": &functions.Function{
							Fn: func(store references.Store) error { return nil },
						},
					},
				},
				Response: &Request{},
			},
		},
		"response functions": {
			node: &specs.Node{},
			options: &CallOptions{
				Request: &Request{},
				Response: &Request{
					functions: functions.Stack{
						"sample": &functions.Function{
							Fn: func(store references.Store) error { return nil },
						},
					},
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := logger.WithLogger(broker.NewContext())

			result := NewCall(ctx, test.node, test.options)
			if result == nil {
				t.Fatal("unexpected result, expected a call to be constructed")
			}

			err := result.Do(context.Background(), test.store)
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCallFunctionsExecution(t *testing.T) {
	type test struct {
		fn       *fncounter
		expected int
		node     *specs.Node
		options  *CallOptions
		store    references.Store
	}

	tests := map[string]func() *test{
		"request": func() *test {
			counter := &fncounter{}

			return &test{
				fn:       counter,
				expected: 1,
				node:     &specs.Node{},
				options: &CallOptions{
					Request: &Request{
						functions: functions.Stack{
							"sample": &functions.Function{
								Fn: counter.handle,
							},
						},
					},
					Response: &Request{},
				},
			}
		},
		"response": func() *test {
			counter := &fncounter{}

			return &test{
				fn:       counter,
				expected: 1,
				node:     &specs.Node{},
				options: &CallOptions{
					Request: &Request{},
					Response: &Request{
						functions: functions.Stack{
							"sample": &functions.Function{
								Fn: counter.handle,
							},
						},
					},
				},
			}
		},
		"multiple": func() *test {
			counter := &fncounter{}

			return &test{
				fn:       counter,
				expected: 4,
				node:     &specs.Node{},
				options: &CallOptions{
					Request: &Request{
						functions: functions.Stack{
							"first": &functions.Function{
								Fn: counter.handle,
							},
							"second": &functions.Function{
								Fn: counter.handle,
							},
						},
					},
					Response: &Request{
						functions: functions.Stack{
							"first": &functions.Function{
								Fn: counter.handle,
							},
							"second": &functions.Function{
								Fn: counter.handle,
							},
						},
					},
				},
			}
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := logger.WithLogger(broker.NewContext())
			options := test()

			result := NewCall(ctx, options.node, options.options)
			if result == nil {
				t.Fatal("unexpected result, expected a call to be constructed")
			}

			err := result.Do(context.Background(), options.store)
			if err != nil {
				t.Fatal(err)
			}

			if options.fn.counter != options.expected {
				t.Fatalf("unexpected result %d, expected %d functions to be called", options.fn.counter, options.expected)
			}
		})
	}
}

func TestCallErrHandling(t *testing.T) {
	expected := errors.New("abort")
	node := &specs.Node{}
	options := &CallOptions{
		Transport: NewMockTransport(expected, 0, nil),
	}

	ctx := logger.WithLogger(broker.NewContext())
	call := NewCall(ctx, node, options)

	store := references.NewReferenceStore(0)
	err := call.Do(context.Background(), store)
	if err == nil {
		t.Fatal("unexpected pass expected a error to be returned")
	}

	if !errors.Is(err, expected) {
		t.Fatalf("unexpected err '%s', exepected '%s'", err, expected)
	}
}

func TestTransportStatusCodeHandling(t *testing.T) {
	type test struct {
		node    *specs.Node
		options *CallOptions
		store   references.Store
		err     error
	}

	tests := map[string]func() *test{
		"default": func() *test {
			return &test{
				node: &specs.Node{},
				options: &CallOptions{
					Transport: NewMockTransport(nil, transport.StatusOK, nil),
				},
				store: references.NewReferenceStore(0),
				err:   nil,
			}
		},
		"200": func() *test {
			return &test{
				node: &specs.Node{},
				options: &CallOptions{
					Transport:      NewMockTransport(nil, transport.StatusOK, nil),
					ExpectedStatus: []int{transport.StatusOK},
				},
				store: references.NewReferenceStore(0),
				err:   nil,
			}
		},
		"201": func() *test {
			return &test{
				node: &specs.Node{},
				options: &CallOptions{
					Transport:      NewMockTransport(nil, 201, nil),
					ExpectedStatus: []int{201},
				},
				store: references.NewReferenceStore(0),
				err:   nil,
			}
		},
		"300": func() *test {
			return &test{
				node: &specs.Node{},
				options: &CallOptions{
					Transport:      NewMockTransport(nil, 300, nil),
					ExpectedStatus: []int{300},
				},
				store: references.NewReferenceStore(0),
				err:   nil,
			}
		},
		"500": func() *test {
			return &test{
				node: &specs.Node{},
				options: &CallOptions{
					Transport:      NewMockTransport(nil, 500, nil),
					ExpectedStatus: []int{transport.StatusOK},
				},
				store: references.NewReferenceStore(0),
				err:   ErrAbortFlow,
			}
		},
		"401": func() *test {
			return &test{
				node: &specs.Node{},
				options: &CallOptions{
					Transport:      NewMockTransport(nil, 401, nil),
					ExpectedStatus: []int{transport.StatusOK},
				},
				store: references.NewReferenceStore(0),
				err:   ErrAbortFlow,
			}
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := logger.WithLogger(broker.NewContext())
			options := test()

			result := NewCall(ctx, options.node, options.options)
			if result == nil {
				t.Fatal("unexpected result, expected a call to be constructed")
			}

			err := result.Do(context.Background(), options.store)
			if err != options.err {
				t.Fatalf("unexpected err '%+v', expected '%+v'", err, options.err)
			}
		})
	}
}

func TestTransportErrorSchemaDecoding(t *testing.T) {
	type test struct {
		node      *specs.Node
		options   *CallOptions
		store     references.Store
		reference string
	}

	tests := map[string]func(t *testing.T) *test{
		"simple": func(t *testing.T) *test {
			message := `{"message": "something went wrong"}`

			constructor := json.NewConstructor()
			codec, err := constructor.New(template.ErrorResource, &specs.ParameterMap{
				Property: &specs.Property{
					Type:  types.Message,
					Label: labels.Optional,
					Nested: map[string]*specs.Property{
						"message": {
							Path:  "message",
							Type:  types.String,
							Label: labels.Optional,
						},
					},
				},
			})

			if err != nil {
				t.Fatal(err)
			}

			return &test{
				node: &specs.Node{},
				options: &CallOptions{
					ExpectedStatus: []int{transport.StatusOK},
					Transport:      NewMockTransport(nil, 500, []byte(message)),
					Err:            NewOnError(nil, codec, nil, nil, nil),
				},
				store:     references.NewReferenceStore(1),
				reference: "message",
			}
		},
		"complex": func(t *testing.T) *test {
			message := `{"meta": { "message": "something went wrong" }}`

			constructor := json.NewConstructor()
			codec, err := constructor.New(template.ErrorResource, &specs.ParameterMap{
				Property: &specs.Property{
					Type:  types.Message,
					Label: labels.Optional,
					Nested: map[string]*specs.Property{
						"meta": {
							Path:  "meta",
							Type:  types.Message,
							Label: labels.Optional,
							Nested: map[string]*specs.Property{
								"message": {
									Path:  "meta.message",
									Type:  types.String,
									Label: labels.Optional,
								},
							},
						},
					},
				},
			})

			if err != nil {
				t.Fatal(err)
			}

			return &test{
				node: &specs.Node{},
				options: &CallOptions{
					ExpectedStatus: []int{transport.StatusOK},
					Transport:      NewMockTransport(nil, 500, []byte(message)),
					Err:            NewOnError(nil, codec, nil, nil, nil),
				},
				store:     references.NewReferenceStore(1),
				reference: "meta.message",
			}
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := logger.WithLogger(broker.NewContext())
			options := test(t)

			result := NewCall(ctx, options.node, options.options)
			if result == nil {
				t.Fatal("unexpected result, expected a call to be constructed")
			}

			err := result.Do(context.Background(), options.store)
			if err != ErrAbortFlow {
				t.Fatalf("unexpected err '%s', expected '%s'", err, ErrAbortFlow)
			}

			ref := options.store.Load(template.ErrorResource, options.reference)
			if ref == nil {
				t.Fatal("expected reference to be defined")
			}
		})
	}
}

func TestErrFunctionsExecution(t *testing.T) {
	type test struct {
		fn       *fncounter
		expected int
		node     *specs.Node
		options  *CallOptions
		store    references.Store
	}

	tests := map[string]func() *test{
		"single": func() *test {
			counter := &fncounter{}

			return &test{
				fn:       counter,
				expected: 1,
				node:     &specs.Node{},
				store:    references.NewReferenceStore(0),
				options: &CallOptions{
					Transport: NewMockTransport(nil, 500, nil),
					Err: &OnError{
						functions: functions.Stack{
							"sample": &functions.Function{
								Fn: counter.handle,
							},
						},
					},
				},
			}
		},
		"multiple": func() *test {
			counter := &fncounter{}

			return &test{
				fn:       counter,
				expected: 3,
				node:     &specs.Node{},
				store:    references.NewReferenceStore(0),
				options: &CallOptions{
					Transport: NewMockTransport(nil, 500, nil),
					Err: &OnError{
						functions: functions.Stack{
							"first": &functions.Function{
								Fn: counter.handle,
							},
							"second": &functions.Function{
								Fn: counter.handle,
							},
							"third": &functions.Function{
								Fn: counter.handle,
							},
						},
					},
				},
			}
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := logger.WithLogger(broker.NewContext())
			options := test()

			result := NewCall(ctx, options.node, options.options)
			if result == nil {
				t.Fatal("unexpected result, expected a call to be constructed")
			}

			err := result.Do(context.Background(), options.store)
			if err != ErrAbortFlow {
				t.Fatalf("unexpected pass, expected abort flow to be returned")
			}

			if options.fn.counter != options.expected {
				t.Fatalf("unexpected result %d, expected %d functions to be called", options.fn.counter, options.expected)
			}
		})
	}
}
