package ide

type IDE struct {
	Identifier string
	Name       string
	Aliases    []string
	OnOpen     func(hostPattern, folderPath, additionalInfo string) error
	OnTestPath func() (string, bool)
}
