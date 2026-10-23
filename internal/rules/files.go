package rules

import (
	"bytes"
	"errors"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

func readRuleFile(path string) (File, error) {
	var out File
	info, err := os.Lstat(path)
	if err != nil {
		return out, err
	}
	if !info.Mode().IsRegular() || info.Size() > 4<<20 {
		return out, errors.New("rule file must be regular and at most 4 MiB")
	}
	file, err := os.Open(path)
	if err != nil {
		return out, err
	}
	defer func() { _ = file.Close() }()
	actual, err := file.Stat()
	if err != nil || !os.SameFile(info, actual) || !actual.Mode().IsRegular() {
		return out, errors.New("rule file changed during inspection")
	}
	body, err := io.ReadAll(io.LimitReader(file, (4<<20)+1))
	if err != nil || len(body) > 4<<20 {
		return out, errors.New("rule file could not be read within its limit")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(body))
	decoder.KnownFields(true)
	if decoder.Decode(&out) != nil || decoder.Decode(new(any)) != io.EOF || len(out.Groups) != 1 {
		return File{}, errors.New("rule file must contain exactly one group in one YAML document")
	}
	return out, nil
}

// Unknown backend options (for example source_tenants) can alter evaluation
// semantics even when the known rules match. Do not discard them during proof
// of identity, overwrite authorization or ambiguous-write readback.
func decodeRuleGroup(body []byte) (Group, error) {
	var group Group
	decoder := yaml.NewDecoder(bytes.NewReader(body))
	decoder.KnownFields(true)
	if decoder.Decode(&group) != nil || decoder.Decode(new(any)) != io.EOF {
		return Group{}, errors.New("rule group contains invalid or unsupported YAML")
	}
	return group, nil
}
