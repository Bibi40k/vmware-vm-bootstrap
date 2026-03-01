package wizard

// Step defines one wizard action.
type Step struct {
	Name string
	Run  func() error
}

// RunSteps executes steps in order and reports transitions through onStepStart.
func RunSteps(steps []Step, onStepStart func(index, total int, name string), onStepDone func(index, total int)) error {
	total := len(steps)
	for i, step := range steps {
		if onStepStart != nil {
			onStepStart(i+1, total, step.Name)
		}
		if step.Run != nil {
			if err := step.Run(); err != nil {
				return err
			}
		}
		if onStepDone != nil {
			onStepDone(i+1, total)
		}
	}
	return nil
}
