package transport

import (
	"github.com/jexia/maestro/internal/codec"
	"github.com/jexia/maestro/pkg/core/instance"
	"github.com/jexia/maestro/pkg/core/logger"
	"github.com/jexia/maestro/pkg/metadata"
	"github.com/jexia/maestro/pkg/refs"
	"github.com/jexia/maestro/pkg/specs"
	"github.com/jexia/maestro/pkg/specs/template"
	"github.com/jexia/maestro/pkg/specs/types"
)

// NewEndpoint constructs a new transport endpoint.
func NewEndpoint(listener string, flow Flow, forward *Forward, options specs.Options, request, response *specs.ParameterMap) *Endpoint {
	result := &Endpoint{
		Listener: listener,
		Flow:     flow,
		Forward:  forward,
		Options:  options,
		Request:  NewObject(request, nil),
		Response: NewObject(response, nil),
	}

	return result
}

// NewObject constructs a new data object
func NewObject(schema *specs.ParameterMap, status *specs.Property) *Object {
	if schema == nil {
		return nil
	}

	return &Object{
		Schema:     schema,
		StatusCode: status,
	}
}

// Object represents a data object.
type Object struct {
	Schema     *specs.ParameterMap
	StatusCode *specs.Property
	Codec      codec.Manager
	Meta       *metadata.Manager
}

// ResolveStatusCode attempts to resolve the defined status code.
// If no status code property has been defined or the property is not a int64.
// Is a internal server error status returned.
func (object *Object) ResolveStatusCode(ctx instance.Context, store refs.Store) int {
	if object.StatusCode == nil {
		return StatusInternalErr
	}

	if object.StatusCode.Type != types.Int64 {
		ctx.Logger(logger.Transport).Error("unexpected status code type '%s', status code has to be a int64", object.StatusCode.Type)
		return StatusInternalErr
	}

	result := object.StatusCode.Default
	if object.StatusCode.Reference != nil {
		ref := store.Load(object.StatusCode.Reference.Resource, object.StatusCode.Reference.Path)
		if ref != nil {
			result = ref.Value
		}
	}

	if result == nil {
		return StatusInternalErr
	}

	return int(result.(int64))
}

// NewMeta updates the current object metadata manager
func (object *Object) NewMeta(ctx instance.Context, resource string) {
	if object == nil || object.Schema == nil {
		return
	}

	object.Meta = metadata.NewManager(ctx, resource, object.Schema.Header)
}

// NewCodec updates the given object to use the given codec.
// Errors returned while constructing a new codec manager are returned.
func (object *Object) NewCodec(ctx instance.Context, resource string, codec codec.Constructor) error {
	if object == nil || object.Schema == nil {
		return nil
	}

	manager, err := codec.New(resource, object.Schema)
	if err != nil {
		return err
	}

	object.Codec = manager
	return nil
}

// Collection represents a collection of requests
type Collection map[*specs.ParameterMap]*Object

// Set appends the given object to the object collection
func (collection Collection) Set(object *Object) {
	if collection == nil {
		return
	}

	collection[object.Schema] = object
}

// Get attempts to retrieve the requested object from the object collection
func (collection Collection) Get(key *specs.ParameterMap) *Object {
	return collection[key]
}

// Errs represents a err object collection
type Errs Collection

// Set appends the given object to the object collection
func (collection Errs) Set(object *Object) {
	if collection == nil {
		return
	}

	collection[object.Schema] = object
}

// Get attempts to retrieve the requested object from the errs collection
func (collection Errs) Get(key Error) *Object {
	return collection[key.GetResponse()]
}

// Forward represents the forward specifications
type Forward struct {
	Schema  specs.Header
	Meta    *metadata.Manager
	Service *specs.Service
}

// NewMeta updates the current object metadata manager
func (forward *Forward) NewMeta(ctx instance.Context, resource string) {
	if forward == nil || forward.Schema == nil {
		return
	}

	forward.Meta = metadata.NewManager(ctx, resource, forward.Schema)
}

// Endpoint represents a transport listener endpoint
type Endpoint struct {
	Listener string
	Flow     Flow
	Forward  *Forward
	Options  specs.Options
	Request  *Object
	Response *Object
	Errs     Errs
}

// NewCodec updates the endpoint request and response codecs and metadata managers.
// If a forwarding service is set is the request codec ignored.
func (endpoint *Endpoint) NewCodec(ctx instance.Context, codec codec.Constructor) (err error) {
	endpoint.Request.NewMeta(ctx, template.InputResource)

	if endpoint.Forward == nil {
		err = endpoint.Request.NewCodec(ctx, template.InputResource, codec)
		if err != nil {
			return err
		}
	}

	for _, handle := range endpoint.Flow.Errors() {
		object := NewObject(handle.GetResponse(), handle.GetStatusCode())

		object.NewMeta(ctx, template.ErrorResource)
		err = object.NewCodec(ctx, template.ErrorResource, codec)
		if err != nil {
			return err
		}

		endpoint.Errs.Set(object)
	}

	endpoint.Response.NewMeta(ctx, template.OutputResource)
	err = endpoint.Response.NewCodec(ctx, template.OutputResource, codec)
	if err != nil {
		return err
	}

	endpoint.Forward.NewMeta(ctx, template.OutputResource)
	return nil
}