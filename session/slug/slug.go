package slug

type Manager interface {
	Get() string
	Set(slug string)
}

type manager struct {
	slug string
}

func NewManager() Manager {
	return &manager{
		slug: "",
	}
}

func (s *manager) Get() string {
	return s.slug
}

func (s *manager) Set(slug string) {
	s.slug = slug
}
