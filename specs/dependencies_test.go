package specs

import (
	"testing"

	"github.com/jexia/maestro/instance"
)

func TestResolveManifestDependencies(t *testing.T) {
	manifest := &Manifest{
		Flows: []*Flow{
			{
				Name: "first",
				Nodes: []*Node{
					{
						Name: "first",
					},
					{
						Name: "second",
						DependsOn: map[string]*Node{
							"first": nil,
						},
					},
					{
						Name: "third",
						DependsOn: map[string]*Node{
							"first":  nil,
							"second": nil,
						},
					},
				},
			},
			{
				Name: "second",
				Nodes: []*Node{
					{
						Name: "first",
					},
					{
						Name: "second",
						DependsOn: map[string]*Node{
							"first": nil,
						},
					},
					{
						Name: "third",
						DependsOn: map[string]*Node{
							"first":  nil,
							"second": nil,
						},
					},
				},
			},
			{
				Name: "third",
				Nodes: []*Node{
					{
						Name: "first",
					},
					{
						Name: "second",
						DependsOn: map[string]*Node{
							"first": nil,
						},
					},
					{
						Name: "third",
						DependsOn: map[string]*Node{
							"first":  nil,
							"second": nil,
						},
					},
				},
			},
		},
	}

	ctx := instance.NewContext()
	err := ResolveManifestDependencies(ctx, manifest)
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
					t.Fatalf("call dependency not resolved %s.%s", call.Name, key)
				}
			}
		}
	}
}

func TestResolveCallDependencies(t *testing.T) {
	flow := &Flow{
		Nodes: []*Node{
			{
				Name: "first",
			},
			{
				Name: "second",
				DependsOn: map[string]*Node{
					"first": nil,
				},
			},
			{
				Name: "third",
				DependsOn: map[string]*Node{
					"first":  nil,
					"second": nil,
				},
			},
		},
	}

	tests := []*Node{
		flow.Nodes[1],
		flow.Nodes[2],
	}

	for _, input := range tests {
		err := ResolveCallDependencies(flow, input, make(map[string]*Node))
		if err != nil {
			t.Fatalf("unexpected error %s", err)
		}

		for key, val := range input.DependsOn {
			if val == nil {
				t.Fatalf("dependency not resolved %s.%s", input.Name, key)
			}
		}
	}
}

func TestCallCircularDependenciesDetection(t *testing.T) {
	flow := &Flow{
		Nodes: []*Node{
			{
				Name: "first",
				DependsOn: map[string]*Node{
					"second": nil,
				},
			},
			{
				Name: "second",
				DependsOn: map[string]*Node{
					"first": nil,
				},
			},
		},
	}

	tests := []*Node{
		flow.Nodes[0],
		flow.Nodes[1],
	}

	for _, input := range tests {
		err := ResolveCallDependencies(flow, input, make(map[string]*Node))
		if err == nil {
			t.Fatalf("unexpected pass %s", input.Name)
		}
	}
}
