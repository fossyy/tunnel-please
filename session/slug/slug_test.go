package slug

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type SlugTestSuite struct {
	suite.Suite
	slug Slug
}

func (suite *SlugTestSuite) SetupTest() {
	suite.slug = New()
}

func TestNew(t *testing.T) {
	s := New()

	assert.NotNil(t, s, "New() should return a non-nil Slug")
	assert.Implements(t, (*Slug)(nil), s, "New() should return a type that implements Slug interface")
	assert.Equal(t, "", s.String(), "New() should initialize with empty string")
}

func (suite *SlugTestSuite) TestString() {
	assert.Equal(suite.T(), "", suite.slug.String(), "String() should return empty string initially")

	suite.slug.Set("test-slug")
	assert.Equal(suite.T(), "test-slug", suite.slug.String(), "String() should return the set value")
}

func (suite *SlugTestSuite) TestSet() {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple slug",
			input:    "hello-world",
			expected: "hello-world",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "slug with numbers",
			input:    "test-123",
			expected: "test-123",
		},
		{
			name:     "slug with special characters",
			input:    "hello_world-123",
			expected: "hello_world-123",
		},
		{
			name:     "overwrite existing slug",
			input:    "new-slug",
			expected: "new-slug",
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.slug.Set(tc.input)
			assert.Equal(suite.T(), tc.expected, suite.slug.String())
		})
	}
}

func (suite *SlugTestSuite) TestMultipleSet() {
	suite.slug.Set("first-slug")
	assert.Equal(suite.T(), "first-slug", suite.slug.String())

	suite.slug.Set("second-slug")
	assert.Equal(suite.T(), "second-slug", suite.slug.String())

	suite.slug.Set("")
	assert.Equal(suite.T(), "", suite.slug.String())
}

func TestSlugInterface(t *testing.T) {
	var _ Slug = (*slug)(nil)
	var _ Slug = New()
}

func TestSlugIsolation(t *testing.T) {
	slug1 := New()
	slug2 := New()

	slug1.Set("slug-one")
	slug2.Set("slug-two")

	assert.Equal(t, "slug-one", slug1.String(), "First slug should maintain its value")
	assert.Equal(t, "slug-two", slug2.String(), "Second slug should maintain its value")
}

func TestSlugTestSuite(t *testing.T) {
	suite.Run(t, new(SlugTestSuite))
}
