package validation

import "testing"

func TestValidateIdentifier(t *testing.T) {
	valid := []string{"av", "VAULT_DIR", "_x", "a1", "deploy_prod"}
	for _, s := range valid {
		if err := ValidateIdentifier(s); err != nil {
			t.Errorf("ValidateIdentifier(%q) = %v, ожидался nil", s, err)
		}
	}
	invalid := []string{"", "  ", "2foo", "my cmd", "a-b", "foo!", "db-migrate"}
	for _, s := range invalid {
		if err := ValidateIdentifier(s); err == nil {
			t.Errorf("ValidateIdentifier(%q) = nil, ожидалась ошибка", s)
		}
	}
}

func TestValidateCommandName(t *testing.T) {
	valid := []string{"note", "db-migrate", "deploy_prod", "a1"}
	for _, s := range valid {
		if err := ValidateCommandName(s); err != nil {
			t.Errorf("ValidateCommandName(%q) = %v, ожидался nil", s, err)
		}
	}
	invalid := []string{"", "  ", "-leading", "my cmd", "foo!", "2cmd"}
	for _, s := range invalid {
		if err := ValidateCommandName(s); err == nil {
			t.Errorf("ValidateCommandName(%q) = nil, ожидалась ошибка", s)
		}
	}
}

func TestValidateFlagName(t *testing.T) {
	valid := []string{"--force", "-f", "build", "--dry-run", "-v"}
	for _, s := range valid {
		if err := ValidateFlagName(s); err != nil {
			t.Errorf("ValidateFlagName(%q) = %v, ожидался nil", s, err)
		}
	}
	invalid := []string{"", "  ", "--", "-", "my flag", "--"}
	for _, s := range invalid {
		if err := ValidateFlagName(s); err == nil {
			t.Errorf("ValidateFlagName(%q) = nil, ожидалась ошибка", s)
		}
	}
}

func TestCheckCondition(t *testing.T) {
	if err := CheckCondition(""); err != nil {
		t.Errorf("пустое условие должно быть валидным, got %v", err)
	}
	if err := CheckCondition("[[ -f docker-compose.yml ]]"); err != nil {
		t.Errorf("корректное условие должно проходить, got %v", err)
	}
	if err := CheckCondition("[[ -f foo "); err == nil {
		t.Errorf("незакрытое условие должно давать ошибку синтаксиса")
	}
}
