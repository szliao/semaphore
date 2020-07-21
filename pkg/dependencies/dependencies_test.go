package dependencies

import (
	"testing"

	"github.com/jexia/semaphore/pkg/core/instance"
	"github.com/jexia/semaphore/pkg/specs"
)

func TestResolveManifestDependencies(t *testing.T) {
	manifest := &specs.FlowsManifest{
		Flows: []*specs.Flow{
			{
				Name: "first",
				Nodes: []*specs.Node{
					{
						ID: "first",
					},
					{
						ID: "second",
						DependsOn: map[string]*specs.Node{
							"first": nil,
						},
					},
					{
						ID: "third",
						DependsOn: map[string]*specs.Node{
							"first":  nil,
							"second": nil,
						},
					},
				},
			},
			{
				Name: "second",
				Nodes: []*specs.Node{
					{
						ID: "first",
					},
					{
						ID: "second",
						DependsOn: map[string]*specs.Node{
							"first": nil,
						},
					},
					{
						ID: "third",
						DependsOn: map[string]*specs.Node{
							"first":  nil,
							"second": nil,
						},
					},
				},
			},
			{
				Name: "third",
				Nodes: []*specs.Node{
					{
						ID: "first",
					},
					{
						ID: "second",
						DependsOn: map[string]*specs.Node{
							"first": nil,
						},
					},
					{
						ID: "third",
						DependsOn: map[string]*specs.Node{
							"first":  nil,
							"second": nil,
						},
					},
				},
			},
		},
		Proxy: []*specs.Proxy{
			{
				Name: "first",
				Nodes: []*specs.Node{
					{
						ID: "first",
					},
					{
						ID: "second",
						DependsOn: map[string]*specs.Node{
							"first": nil,
						},
					},
					{
						ID: "third",
						DependsOn: map[string]*specs.Node{
							"first":  nil,
							"second": nil,
						},
					},
				},
			},
			{
				Name: "second",
				Nodes: []*specs.Node{
					{
						ID: "first",
					},
					{
						ID: "second",
						DependsOn: map[string]*specs.Node{
							"first": nil,
						},
					},
					{
						ID: "third",
						DependsOn: map[string]*specs.Node{
							"first":  nil,
							"second": nil,
						},
					},
				},
			},
			{
				Name: "third",
				Nodes: []*specs.Node{
					{
						ID: "first",
					},
					{
						ID: "second",
						DependsOn: map[string]*specs.Node{
							"first": nil,
						},
					},
					{
						ID: "third",
						DependsOn: map[string]*specs.Node{
							"first":  nil,
							"second": nil,
						},
					},
				},
			},
		},
	}

	ctx := instance.NewContext()
	err := ResolveManifest(ctx, manifest)
	if err != nil {
		t.Fatalf("unexpected error %s", err)
	}

	for _, flow := range manifest.Flows {
		for _, call := range flow.Nodes {
			if len(call.DependsOn) == 0 {
				continue
			}

			for key, val := range call.DependsOn {
				if val == nil {
					t.Fatalf("call dependency not resolved %s.%s", call.ID, key)
				}
			}
		}
	}
}

func TestResolveCallDependencies(t *testing.T) {
	flow := &specs.Flow{
		Nodes: []*specs.Node{
			{
				ID: "first",
			},
			{
				ID: "second",
				DependsOn: map[string]*specs.Node{
					"first": nil,
				},
			},
			{
				ID: "third",
				DependsOn: map[string]*specs.Node{
					"first":  nil,
					"second": nil,
				},
			},
		},
	}

	tests := []*specs.Node{
		flow.Nodes[1],
		flow.Nodes[2],
	}

	for _, input := range tests {
		err := ResolveNode(flow, input, make(map[string]*specs.Node))
		if err != nil {
			t.Fatalf("unexpected error %s", err)
		}

		for key, val := range input.DependsOn {
			if val == nil {
				t.Fatalf("dependency not resolved %s.%s", input.ID, key)
			}
		}
	}
}

func TestCallCircularDependenciesDetection(t *testing.T) {
	flow := &specs.Flow{
		Nodes: []*specs.Node{
			{
				ID: "first",
				DependsOn: map[string]*specs.Node{
					"second": nil,
				},
			},
			{
				ID: "second",
				DependsOn: map[string]*specs.Node{
					"first": nil,
				},
			},
		},
	}

	tests := []*specs.Node{
		flow.Nodes[0],
		flow.Nodes[1],
	}

	for _, input := range tests {
		err := ResolveNode(flow, input, make(map[string]*specs.Node))
		if err == nil {
			t.Fatalf("unexpected pass %s", input.ID)
		}
	}
}

func TestSelfDependencyDetection(t *testing.T) {
	flow := &specs.Flow{
		Nodes: []*specs.Node{
			{
				ID: "first",
				DependsOn: map[string]*specs.Node{
					"first": nil,
				},
			},
			{
				ID: "second",
				DependsOn: map[string]*specs.Node{
					"second": nil,
				},
			},
		},
	}

	tests := []*specs.Node{
		flow.Nodes[0],
	}

	for _, input := range tests {
		err := ResolveNode(flow, input, make(map[string]*specs.Node))
		if err != nil {
			t.Fatalf("unexpected error %s", err)
		}

		if len(input.DependsOn) > 0 {
			t.Fatalf("unexpted remaining dependencies, expected dependencies to be empty: %+v", input.DependsOn)
		}
	}
}