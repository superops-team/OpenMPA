package session

func (s *Service) markTraceSession(id string) {
	if id == "" {
		return
	}
	s.traceMu.Lock()
	defer s.traceMu.Unlock()
	s.traceIDs[id] = true
}

func (s *Service) traceSession(id string) bool {
	s.traceMu.Lock()
	defer s.traceMu.Unlock()
	return s.traceIDs[id]
}

func (s *Service) setTraceTitle(id, title string) {
	if id == "" {
		return
	}
	s.traceMu.Lock()
	defer s.traceMu.Unlock()
	s.traceTitles[id] = title
}

func (s *Service) traceTitle(id string) string {
	s.traceMu.Lock()
	defer s.traceMu.Unlock()
	return s.traceTitles[id]
}
