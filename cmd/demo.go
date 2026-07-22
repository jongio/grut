package cmd

import (
	"fmt"
	"io"

	"github.com/jongio/grut/internal/demo"
)

func setupDemoProjectWithOptions(scenario string, keep bool) (*demo.ScenarioSetup, func(), error) {
	return demo.SetupProjectWithOptions(demo.SetupOptions{Scenario: scenario, Keep: keep})
}

func printDemoScenarioList(w io.Writer) error {
	_, err := fmt.Fprint(w, demo.FormatScenarioList())
	return err
}

func isDemoScenarioList(name string) bool {
	return demo.IsScenarioListName(name)
}
