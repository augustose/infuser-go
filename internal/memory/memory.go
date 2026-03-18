package memory

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/augustose/infuser-go/internal/parser"
)

type LocalMemory struct {
	stateFile string
	State     *parser.DesiredState
}

func NewMemory(stateFile string) *LocalMemory {
	return &LocalMemory{
		stateFile: stateFile,
		State:     parser.NewDesiredState(),
	}
}

func (m *LocalMemory) Load() error {
	data, err := os.ReadFile(m.stateFile)
	if os.IsNotExist(err) {
		return nil // empty state is fine
	}
	if err != nil {
		return err
	}

	var state parser.DesiredState
	if err := json.Unmarshal(data, &state); err != nil {
		fmt.Println("WARNING: Corrupted state file. Using empty state.")
		return nil
	}

	if state.Users == nil {
		state.Users = make(map[string]map[string]any)
	}
	if state.Organizations == nil {
		state.Organizations = make(map[string]map[string]any)
	}

	m.State = &state
	return nil
}

func (m *LocalMemory) Save(desired *parser.DesiredState) error {
	data, err := json.MarshalIndent(desired, "", "    ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(m.stateFile, data, 0644); err != nil {
		return err
	}

	fmt.Println("Local state successfully saved.")
	return nil
}
