package cook

import "testing"

// Each row records the backend's real argument type, so that a .kcpps editor cannot
// offer a control that writes a value the backend rejects. The "why" column is the
// upstream declaration these were checked against.
func TestOptionValueTypesMatchBackendArguments(t *testing.T) {
	cases := []struct {
		key       string
		valueType string
		why       string
	}{
		{"smartcache", ValueNumber, "koboldcpp --smartcache: type=int, const=1, default=0 (KV cache slot limit)"},
		{"moecpu", ValueNumber, "koboldcpp --moecpu: type=int, const=999, default=0 (CPU layer count)"},
		{"sdclamped", ValueNumber, "koboldcpp --sdclamped: type=int, const=512, default=0 (max resolution)"},
		{"sdclampedsoft", ValueNumber, "koboldcpp --sdclampedsoft: type=int, default=0 (max resolution)"},
		{"multiuser", ValueNumber, "koboldcpp --multiuser: type=int (queued request limit)"},
		{"debugmode", ValueNumber, "koboldcpp --debugmode: type=int, levels -1/0/1"},
		{"maingpu", ValueNumber, "koboldcpp --maingpu: type=int, default=-1 (device index)"},
		{"loramult", ValueNumber, "koboldcpp --loramult: type=float, default=1.0 (scalar)"},
		{"usevulkan", ValueJSON, "koboldcpp --usevulkan: nargs='*', type=int (device id list)"},
		{"usecuda", ValueJSON, "koboldcpp --usecuda: nargs='*' with string choices"},
		{"ssl", ValueJSON, "koboldcpp --ssl: nargs='+' (cert_pem, key_pem)"},
		{"routermode", ValueBool, "koboldcpp --routermode: action='store_true'"},
		{"sdstreamlayers", ValueBool, "stable-diffusion.cpp --stream-layers: declared in bool_options, takes no value"},

		// Guard the two that look like this class but are genuinely numeric, so a future
		// sweep does not "fix" them into the sd.cpp spelling of the same idea.
		{"sdquant", ValueNumber, "koboldcpp --sdquant: type=int, choices=[0,1,2]"},
		{"sdtiledvae", ValueNumber, "koboldcpp --sdtiledvae: type=int (tile threshold, e.g. 768)"},
	}
	for _, testCase := range cases {
		definition, ok := OptionDefinitionForKey(testCase.key)
		if !ok {
			t.Errorf("%s: not present in the option catalog", testCase.key)
			continue
		}
		if definition.ValueType != testCase.valueType {
			t.Errorf("%s: value_type = %q, want %q (%s)", testCase.key, definition.ValueType, testCase.valueType, testCase.why)
		}
	}
}

// A non-boolean option must not offer true/false as its choices, or the editor presents a
// control that can only produce invalid values.
func TestNonBooleanOptionsDoNotOfferBooleanChoices(t *testing.T) {
	for _, definition := range OptionCatalog() {
		if definition.ValueType == ValueBool || len(definition.Choices) != 2 {
			continue
		}
		if definition.Choices[0] == "true" && definition.Choices[1] == "false" {
			t.Errorf("%s: value_type %q but choices are true/false", definition.Key, definition.ValueType)
		}
	}
}
