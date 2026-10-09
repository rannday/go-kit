package envx

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

const dotEnvMaxLine = 1024 * 1024

func parseDotEnv(path string) (values map[string]string, retErr error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read dotenv %q: %w", path, err)
	}
	defer func() {
		if err := file.Close(); err != nil && retErr == nil {
			retErr = fmt.Errorf("read dotenv %q: %w", path, err)
		}
	}()

	values = make(map[string]string)

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), dotEnvMaxLine)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if lineNum == 1 {
			line = strings.TrimPrefix(line, "\ufeff")
		}

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("dotenv %s:%d: invalid line: %q", path, lineNum, line)
		}

		key := strings.TrimSpace(parts[0])
		key = strings.TrimPrefix(key, "export ")
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("dotenv %s:%d: invalid line: %q", path, lineNum, line)
		}

		value := strings.TrimSpace(parts[1])
		value = stripInlineComment(value)

		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}

		values[key] = value
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read dotenv %q: %w", path, err)
	}

	return values, nil
}

func stripInlineComment(value string) string {
	inSingle := false
	inDouble := false

	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '#':
			if !inSingle && !inDouble {
				if i == 0 {
					return ""
				}
				if value[i-1] == ' ' || value[i-1] == '\t' {
					return strings.TrimSpace(value[:i])
				}
				if onlyWhitespace(value[:i]) {
					return strings.TrimSpace(value[:i])
				}
			}
		}
	}

	return strings.TrimSpace(value)
}

func onlyWhitespace(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' && s[i] != '\t' {
			return false
		}
	}
	return true
}
