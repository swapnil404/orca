package reconciler

import "testing"

func TestDiffReplicas(t *testing.T) {
	desired := []*ReplicaSpec{{Id: "1"}}
	tests := []struct {
		name   string
		actual []*ActualReplica
		want   []ActionType
	}{
		{name: "create missing", want: []ActionType{ActionCreateReplica}},
		{name: "keep running", actual: []*ActualReplica{{Id: "1", Status: "running"}}},
		{name: "recreate stopped", actual: []*ActualReplica{{Id: "1", Status: "stopped"}}, want: []ActionType{ActionDeleteReplica, ActionCreateReplica}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actions := diffReplicas("cluster", desired, test.actual)
			if len(actions) != len(test.want) {
				t.Fatalf("got %d actions, want %d: %#v", len(actions), len(test.want), actions)
			}
			for index, want := range test.want {
				if actions[index].Type != want {
					t.Errorf("action %d type = %q, want %q", index, actions[index].Type, want)
				}
			}
		})
	}
}
