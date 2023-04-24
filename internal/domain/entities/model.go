package entities

type Model struct {
	Name      string
	MaxTokens int
}

func NewModel(name string, maxTokens int) *Model {
	return &Model{
		Name:      name,
		MaxTokens: maxTokens,
	}
}

func (model *Model) GetMaxTokens() int {
	return model.MaxTokens
}

func (model *Model) GetModelName() string {
	return model.Name
}
