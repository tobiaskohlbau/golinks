package server

func (s *server) Routes() {
	s.r.Get("/registryz", s.handleRegistry())
	s.r.Get("/edit/*", s.handleEdit())
	s.r.Post("/apiz/save", s.handleSave())
	s.r.Get("/*", s.handleRedirect())
}
