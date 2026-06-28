package participants

import (
	"os"
	"fmt"

	"gopkg.in/yaml.v3"
)

type Profile struct {
	ID       string `yaml:"id"`
	Name     string `yaml:"name"`
	Email    string `yaml:"email"`
	Telegram string `yaml:"telegram"`
	Delivery string `yaml:"delivery"` // file, telegram, email
}

type ProfilesConfig struct {
	Profiles []Profile `yaml:"profiles"`
}

// LoadProfiles читает YAML-файл профилей и возвращает срез профилей
func LoadProfiles(path string) ([]Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read profiles file: %w", err)
	}
	var cfg ProfilesConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse profiles: %w", err)
	}
	if len(cfg.Profiles) == 0 {
		return nil, fmt.Errorf("no profiles defined")
	}
	// Проверяем наличие профиля default
	hasDefault := false
	for _, p := range cfg.Profiles {
		if p.ID == "default" {
			hasDefault = true
			break
		}
	}
	if !hasDefault {
		// добавляем автоматически
		cfg.Profiles = append(cfg.Profiles, Profile{
			ID:       "default",
			Name:     "Неизвестный участник",
			Delivery: "file",
		})
	}
	return cfg.Profiles, nil
}