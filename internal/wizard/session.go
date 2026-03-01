package wizard

// Session manages a wizard draft lifecycle with pluggable IO handlers.
type Session struct {
	TargetPath string
	DraftPath  string
	State      any
	IsEmpty    func() bool

	loadDraftFn func(draftPath string, state any) (bool, error)
	startFn     func(targetPath, draftPath string, state any, isEmpty func() bool) func()
	finalizeFn  func(targetPath string) error

	stopFn func()
}

// NewSession creates a reusable wizard session orchestrator.
func NewSession(
	targetPath, draftPath string,
	state any,
	isEmpty func() bool,
	loadDraftFn func(draftPath string, state any) (bool, error),
	startFn func(targetPath, draftPath string, state any, isEmpty func() bool) func(),
	finalizeFn func(targetPath string) error,
) *Session {
	return &Session{
		TargetPath:  targetPath,
		DraftPath:   draftPath,
		State:       state,
		IsEmpty:     isEmpty,
		loadDraftFn: loadDraftFn,
		startFn:     startFn,
		finalizeFn:  finalizeFn,
	}
}

// LoadDraft loads draft state when available.
func (s *Session) LoadDraft() (bool, error) {
	if s == nil || s.loadDraftFn == nil {
		return false, nil
	}
	return s.loadDraftFn(s.DraftPath, s.State)
}

// Start installs draft handling for this session.
func (s *Session) Start() {
	if s == nil || s.startFn == nil {
		return
	}
	s.stopFn = s.startFn(s.TargetPath, s.DraftPath, s.State, s.IsEmpty)
}

// Stop restores process state after Start.
func (s *Session) Stop() {
	if s == nil || s.stopFn == nil {
		return
	}
	s.stopFn()
	s.stopFn = nil
}

// Finalize removes stale drafts for target.
func (s *Session) Finalize() error {
	if s == nil || s.finalizeFn == nil {
		return nil
	}
	return s.finalizeFn(s.TargetPath)
}
