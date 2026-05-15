package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

func extractTranscriptTextFromJSON(input []byte) (string, error) {
	var output strings.Builder
	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.UseNumber()

	if err := collectTextValues(decoder, &output); err != nil {
		return "", err
	}

	if token, err := decoder.Token(); err != io.EOF {
		if err != nil {
			return "", err
		}
		return "", fmt.Errorf("unexpected extra JSON token %v", token)
	}

	return output.String(), nil
}

func collectTextValues(decoder *json.Decoder, output *strings.Builder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}

	return collectToken(decoder, output, token, false)
}

func collectToken(decoder *json.Decoder, output *strings.Builder, token json.Token, collect bool) error {
	switch value := token.(type) {
	case json.Delim:
		switch value {
		case '{':
			return collectObject(decoder, output)
		case '[':
			return collectArray(decoder, output)
		default:
			return fmt.Errorf("unexpected delimiter %q", value)
		}
	case string:
		if collect {
			appendText(output, value)
		}
	default:
		return nil
	}

	return nil
}

func collectObject(decoder *json.Decoder, output *strings.Builder) error {
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return err
		}

		key, ok := token.(string)
		if !ok {
			return fmt.Errorf("expected object key, got %T", token)
		}

		if err := collectValue(decoder, output, key == "text"); err != nil {
			return err
		}
	}

	return consumeEnd(decoder, '}')
}

func collectArray(decoder *json.Decoder, output *strings.Builder) error {
	for decoder.More() {
		if err := collectValue(decoder, output, false); err != nil {
			return err
		}
	}

	return consumeEnd(decoder, ']')
}

func collectValue(decoder *json.Decoder, output *strings.Builder, collect bool) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}

	return collectToken(decoder, output, token, collect)
}

func consumeEnd(decoder *json.Decoder, expected json.Delim) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}

	delim, ok := token.(json.Delim)
	if !ok || delim != expected {
		return fmt.Errorf("expected delimiter %q, got %v", expected, token)
	}

	return nil
}

func appendText(output *strings.Builder, text string) {
	text = strings.ReplaceAll(text, `\n`, "\n")
	if text == "" {
		return
	}

	if output.Len() > 0 && !strings.HasSuffix(output.String(), "\n") {
		output.WriteByte('\n')
	}
	output.WriteString(text)
}
