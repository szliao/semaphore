package flow

import (
	"context"

	"github.com/jexia/semaphore/pkg/broker"
	"github.com/jexia/semaphore/pkg/broker/logger"
	"github.com/jexia/semaphore/pkg/references"
	"github.com/jexia/semaphore/pkg/specs"
	"github.com/jexia/semaphore/pkg/transport"
	"go.uber.org/zap"
)

// NewNode constructs a new node for the given call.
// The service called inside the call endpoint is retrieved from the services collection.
// The call, codec and rollback are defined inside the node and used while processing requests.
func NewNode(parent *broker.Context, node *specs.Node, condition *Condition, call, rollback Call, middleware *NodeMiddleware) *Node {
	module := broker.WithModule(broker.Child(parent), "node", node.Name)
	ctx := logger.WithLogger(module)

	refs := references.References{}

	if middleware == nil {
		middleware = &NodeMiddleware{}
	}

	if node.Call != nil {
		refs.MergeLeft(references.ParameterReferences(node.Call.Request))
	}

	if call != nil {
		for _, prop := range call.References() {
			refs.MergeLeft(references.PropertyReferences(prop))
		}
	}

	if rollback != nil {
		for _, prop := range rollback.References() {
			refs.MergeLeft(references.PropertyReferences(prop))
		}
	}

	return &Node{
		BeforeDo:     middleware.BeforeDo,
		BeforeRevert: middleware.BeforeRollback,
		Condition:    condition,
		ctx:          ctx,
		Name:         node.ID,
		Previous:     []*Node{},
		Call:         call,
		Revert:       rollback,
		DependsOn:    node.DependsOn,
		References:   refs,
		Next:         []*Node{},
		OnError:      node.GetOnError(),
		AfterDo:      middleware.AfterDo,
		AfterRevert:  middleware.AfterRollback,
	}
}

// Nodes represents a node collection
type Nodes []*Node

// Has checks whether the given node collection has a node with the given name inside
func (nodes Nodes) Has(name string) bool {
	for _, node := range nodes {
		if node.Name == name {
			return true
		}
	}

	return false
}

// NodeMiddleware holds all the available
type NodeMiddleware struct {
	BeforeDo       BeforeNode
	AfterDo        AfterNode
	BeforeRollback BeforeNode
	AfterRollback  AfterNode
}

// BeforeNode is called before a node is executed
type BeforeNode func(ctx context.Context, node *Node, tracker *Tracker, processes *Processes, store references.Store) (context.Context, error)

// BeforeNodeHandler wraps the before node function to allow middleware to be chained
type BeforeNodeHandler func(BeforeNode) BeforeNode

// AfterNode is called after a node is executed
type AfterNode func(ctx context.Context, node *Node, tracker *Tracker, processes *Processes, store references.Store) (context.Context, error)

// AfterNodeHandler wraps the after node function to allow middleware to be chained
type AfterNodeHandler func(AfterNode) AfterNode

// Node represents a collection of callers and rollbacks which could be executed parallel.
type Node struct {
	BeforeDo     BeforeNode
	BeforeRevert BeforeNode
	Condition    *Condition
	ctx          *broker.Context
	Name         string
	Previous     Nodes
	Call         Call
	Revert       Call
	DependsOn    map[string]*specs.Node
	References   map[string]*specs.PropertyReference
	Next         Nodes
	OnError      specs.ErrorHandle
	AfterDo      AfterNode
	AfterRevert  AfterNode
}

// Do executes the given node an calls the next nodes.
// If one of the nodes fails is the error marked and are the processes aborted.
func (node *Node) Do(ctx context.Context, tracker *Tracker, processes *Processes, refs references.Store) {
	defer processes.Done()
	logger.Debug(node.ctx, "executing node call")

	tracker.Lock(node)
	defer tracker.Unlock(node)

	if !tracker.Reached(node, len(node.Previous)) {
		logger.Debug(node.ctx, "has not met dependencies yet")
		return
	}

	var err error

	if node.Condition != nil {
		logger.Debug(node.ctx, "evaluating condition")

		pass, err := node.Condition.Eval(node.ctx, refs)
		if err != nil {
			logger.Error(node.ctx, "condition evaluation failed", zap.Error(err))
			processes.Fatal(transport.WrapError(err, node.OnError))
			return
		}

		if !pass {
			logger.Debug(node.ctx, "condition prevented node from being executed")
			node.Skip(ctx, tracker)
			return
		}

		logger.Debug(node.ctx, "node condition passed")
	}

	if node.BeforeDo != nil {
		ctx, err = node.BeforeDo(ctx, node, tracker, processes, refs)
		if err != nil {
			logger.Error(node.ctx, "node before middleware failed", zap.Error(err))
			processes.Fatal(transport.WrapError(err, node.OnError))
			return
		}
	}

	if node.Call != nil {
		err = node.Call.Do(ctx, refs)
		if err != nil {
			logger.Error(node.ctx, "call failed", zap.Error(err))
			processes.Fatal(transport.WrapError(err, node.OnError))
			return
		}
	}

	logger.Debug(node.ctx, "marking node as completed")
	tracker.Mark(node)

	if processes.Err() != nil {
		logger.Error(node.ctx, "stopping execution a error has been thrown", zap.Error(err))
		return
	}

	processes.Add(len(node.Next))
	for _, next := range node.Next {
		tracker.Mark(next)
		go next.Do(ctx, tracker, processes, refs)
	}

	if node.AfterDo != nil {
		_, err = node.AfterDo(ctx, node, tracker, processes, refs)
		if err != nil {
			logger.Error(node.ctx, "node after middleware failed", zap.Error(err))
			processes.Fatal(transport.WrapError(err, node.OnError))
			return
		}
	}
}

// Rollback executes the given node rollback an calls the previous nodes.
// If one of the nodes fails is the error marked but execution is not aborted.
func (node *Node) Rollback(ctx context.Context, tracker *Tracker, processes *Processes, refs references.Store) {
	defer processes.Done()
	logger.Debug(node.ctx, "executing node revert")

	tracker.Lock(node)
	defer tracker.Unlock(node)

	if !tracker.Reached(node, len(node.Next)) {
		logger.Debug(node.ctx, "has not met dependencies yet")
		return
	}

	var err error

	if node.BeforeRevert != nil {
		ctx, err = node.BeforeRevert(ctx, node, tracker, processes, refs)
		if err != nil {
			logger.Error(node.ctx, "node before revert middleware failed", zap.Error(err))
			processes.Fatal(transport.WrapError(err, node.OnError))
			return
		}
	}

	defer func() {
		processes.Add(len(node.Previous))
		for _, node := range node.Previous {
			tracker.Mark(node)
			go node.Rollback(ctx, tracker, processes, refs)
		}
	}()

	if node.Revert != nil {
		err = node.Revert.Do(ctx, refs)
		if err != nil {
			logger.Error(node.ctx, "node revert failed", zap.Error(err))
		}
	}

	logger.Debug(node.ctx, "marking node as completed")
	tracker.Mark(node)

	if node.AfterRevert != nil {
		ctx, err = node.AfterRevert(ctx, node, tracker, processes, refs)
		if err != nil {
			logger.Error(node.ctx, "node after revert middleware failed", zap.Error(err))
			processes.Fatal(transport.WrapError(err, node.OnError))
			return
		}
	}
}

// Skip skips the given node and all it's dependencies and nested conditions
func (node *Node) Skip(ctx context.Context, tracker *Tracker) {
	tracker.Skip(node)

	for _, node := range node.Next {
		if node.Condition != nil {
			node.Skip(ctx, tracker)
			continue
		}

		tracker.Skip(node)
	}
}

// Walk iterates over all nodes and returns the lose ends nodes
func (node *Node) Walk(result map[string]*Node, fn func(node *Node)) {
	fn(node)

	if len(node.Next) == 0 {
		result[node.Name] = node
	}

	for _, next := range node.Next {
		next.Walk(result, fn)
	}
}
