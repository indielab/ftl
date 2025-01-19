package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Project      Project      `yaml:"project" validate:"required"`
	Server       Server       `yaml:"server" validate:"required"`
	Services     []Service    `yaml:"services" validate:"required,dive"`
	Dependencies []Dependency `yaml:"dependencies" validate:"dive"`
	Volumes      []string     `yaml:"volumes" validate:"dive"`
}

type Project struct {
	Name   string `yaml:"name" validate:"required"`
	Domain string `yaml:"domain" validate:"required,fqdn"`
	Email  string `yaml:"email" validate:"required,email"`
}

type Server struct {
	Host       string `yaml:"host" validate:"required,fqdn|ip"`
	Port       int    `yaml:"port" validate:"required,min=1,max=65535"`
	User       string `yaml:"user" validate:"required"`
	Passwd     string `yaml:"-"`
	SSHKey     string `yaml:"ssh_key" validate:"required,filepath"`
	RootSSHKey string `yaml:"-"`
}

type Service struct {
	Name         string `yaml:"name" validate:"required"`
	Image        string `yaml:"image"`
	ImageUpdated bool
	Port         int                 `yaml:"port" validate:"required,min=1,max=65535"`
	Path         string              `yaml:"path"`
	HealthCheck  *ServiceHealthCheck `yaml:"health_check"`
	Routes       []Route             `yaml:"routes" validate:"required,dive"`
	Volumes      []string            `yaml:"volumes" validate:"dive,volume_reference"`
	Command      string              `yaml:"command"`
	Entrypoint   []string            `yaml:"entrypoint"`
	Env          []string            `yaml:"env"`
	Forwards     []string            `yaml:"forwards"`
	Recreate     bool                `yaml:"recreate"`
	Hooks        *Hooks              `yaml:"hooks"`
	Container    *Container          `yaml:"container"`
	LocalPorts   []int               `yaml:"-"`
}

type ServiceHealthCheck struct {
	Path     string        `yaml:"path"`
	Interval time.Duration `yaml:"interval"`
	Timeout  time.Duration `yaml:"timeout"`
	Retries  int           `yaml:"retries"`
}

type Container struct {
	HealthCheck *ContainerHealthCheck `yaml:"health_check"`
	ULimits     []ULimit              `yaml:"ulimits"`
}

type ULimit struct {
	Name string `yaml:"name"`
	Hard int    `yaml:"hard"`
	Soft int    `yaml:"soft"`
}

type ContainerHealthCheck struct {
	Cmd          string `yaml:"cmd"`
	Interval     string `yaml:"interval"`
	Retries      int    `yaml:"retries"`
	Timeout      string `yaml:"timeout"`
	StartPeriod  string `yaml:"start_period"`
	StartTimeout string `yaml:"start_timeout"`
}

type Route struct {
	PathPrefix  string `yaml:"path" validate:"required"`
	StripPrefix bool   `yaml:"strip_prefix"`
}

type Dependency struct {
	Name      string     `yaml:"name" validate:"required"`
	Image     string     `yaml:"image" validate:"required"`
	Volumes   []string   `yaml:"volumes" validate:"dive,volume_reference"`
	Env       []string   `yaml:"env" validate:"dive"`
	Ports     []int      `yaml:"ports" validate:"dive,min=1,max=65535"`
	Container *Container `yaml:"container"`
}

type Volume struct {
	Name string `yaml:"name" validate:"required"`
	Path string `yaml:"path" validate:"required,unix_path"`
}

type Hooks struct {
	Pre  string `yaml:"pre"`
	Post string `yaml:"post"`
}

func ParseConfig(data []byte) (*Config, error) {
	// Load any .env file from the current directory
	_ = godotenv.Load()

	// Process environment variables with default values
	expandedData := os.Expand(string(data), func(key string) string {
		// Check if there's a default value specified
		parts := strings.SplitN(key, ":-", 2)
		envKey := parts[0]

		if value, exists := os.LookupEnv(envKey); exists {
			return value
		}

		// Return default value if specified, empty string otherwise
		if len(parts) > 1 {
			return parts[1]
		}
		return ""
	})

	var config Config
	if err := yaml.Unmarshal([]byte(expandedData), &config); err != nil {
		return nil, fmt.Errorf("error parsing YAML: %v", err)
	}

	// Process .env files for services if they exist
	for i := range config.Services {
		if config.Services[i].Path == "" {
			config.Services[i].Path = "./"
		}

		envPath := filepath.Join(config.Services[i].Path, ".env")
		if _, err := os.Stat(envPath); err == nil {
			if err := godotenv.Load(envPath); err != nil {
				return nil, fmt.Errorf("failed to read .env file: %w", err)
			}
		}
	}

	validate := validator.New()

	_ = validate.RegisterValidation("volume_reference", func(fl validator.FieldLevel) bool {
		value := fl.Field().String()
		parts := strings.Split(value, ":")
		return len(parts) == 2 && parts[0] != "" && parts[1] != ""
	})

	_ = validate.RegisterValidation("unix_path", func(fl validator.FieldLevel) bool {
		value := fl.Field().String()
		return strings.HasPrefix(value, "/")
	})

	if err := validate.Struct(config); err != nil {
		return nil, fmt.Errorf("validation error: %v", err)
	}

	return &config, nil
}

func (s *Service) Hash() (string, error) {
	sortedService := s.sortServiceFields()
	bytes, err := json.Marshal(sortedService)
	if err != nil {
		return "", fmt.Errorf("failed to marshal sorted service: %w", err)
	}

	hash := sha256.Sum256(bytes)
	return hex.EncodeToString(hash[:]), nil
}

func (s *Service) sortServiceFields() map[string]interface{} {
	sorted := make(map[string]interface{})
	v := reflect.ValueOf(*s)
	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		value := v.Field(i).Interface()

		switch reflect.TypeOf(value).Kind() {
		case reflect.Slice:
			s := reflect.ValueOf(value)
			sorted[field.Name] = sortSlice(s)
		case reflect.Map:
			m := reflect.ValueOf(value)
			sorted[field.Name] = sortMap(m)
		default:
			sorted[field.Name] = value
		}
	}

	return sorted
}

func sortSlice(s reflect.Value) []interface{} {
	sorted := make([]interface{}, s.Len())
	for i := 0; i < s.Len(); i++ {
		sorted[i] = s.Index(i).Interface()
	}
	sort.Slice(sorted, func(i, j int) bool {
		return fmt.Sprintf("%v", sorted[i]) < fmt.Sprintf("%v", sorted[j])
	})
	return sorted
}

func sortMap(m reflect.Value) map[string]interface{} {
	sorted := make(map[string]interface{})
	for _, key := range m.MapKeys() {
		sorted[key.String()] = m.MapIndex(key).Interface()
	}
	return sorted
}
