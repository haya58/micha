package persona

import (
	"embed"
	"os"
)

//go:embed default.md
var embedded embed.FS

func Load(customPath string) (string, error) {
	if customPath != "" {
		b, err := os.ReadFile(customPath)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	b, err := embedded.ReadFile("default.md")
	if err != nil {
		return "", err
	}
	return string(b), nil
}
