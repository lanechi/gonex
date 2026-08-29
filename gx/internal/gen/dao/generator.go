package dao

const (
	defaultModelOutput  = "internal/dao"
	defaultEntityOutput = "internal/model/entity"
)

// ModelOptions controls database-to-GORM code generation. Output paths are
// project-relative and intentionally follow the init project layout.
type ModelOptions struct {
	Tables  string
	runTidy func(string) error
}
