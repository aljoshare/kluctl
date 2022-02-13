package types

import "github.com/go-playground/validator/v10"

type SecretSourceAwsSecretsManager struct {
	// Name or ARN of the secret. In case a name is given, the region must be specified as well
	SecretName string `yaml:"path" validate:"required"`
	// The aws region
	Region string `yaml:"path,omitempty"`
	// AWS credentials profile to use. The AWS_PROFILE environemnt variables will take predence in case it is also set
	Profile string `yaml:"path,omitempty"`
}

type SecretSource struct {
	Path              *string                        `yaml:"path,omitempty"`
	AwsSecretsManager *SecretSourceAwsSecretsManager `yaml:"awsSecretsManager,omitempty"`
}

func ValidateSecretSource(sl validator.StructLevel) {
	s := sl.Current().Interface().(SecretSource)
	count := 0
	if s.Path != nil {
		count += 1
	}
	if s.AwsSecretsManager != nil {
		count += 1
	}
	if count == 0 {
		sl.ReportError(s, "self", "self", "invalidsource", "unknown secret source type")
	} else if count != 1 {
		sl.ReportError(s, "self", "self", "invalidsource", "more then one secret source type")
	}
}