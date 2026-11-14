package config

// SSL configures the host certificate and key files.
type SSL struct {
	KeyStr string
	Pem    string
	Enable bool
	Domain string
}
