package slug

type Slug interface {
	String() string
	Set(slug string)
}

type slug struct {
	slug string
}

func New() Slug {
	return &slug{
		slug: "",
	}
}

func (s *slug) String() string {
	return s.slug
}

func (s *slug) Set(slug string) {
	s.slug = slug
}
