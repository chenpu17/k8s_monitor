package i18n

import (
	"embed"
	"encoding/json"

	"github.com/BurntSushi/toml"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

//go:embed *.toml
var localeFS embed.FS

// Bundle holds all translation files
var bundle *i18n.Bundle

func init() {
	// Initialize bundle with default language (English)
	bundle = i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)

	// Load translation files from embedded FS
	// English
	if data, err := localeFS.ReadFile("active.en.toml"); err == nil {
		if _, err := bundle.ParseMessageFileBytes(data, "active.en.toml"); err != nil {
			panic("failed to load English translations: " + err.Error())
		}
	}

	// Chinese (Simplified)
	if data, err := localeFS.ReadFile("active.zh.toml"); err == nil {
		if _, err := bundle.ParseMessageFileBytes(data, "active.zh.toml"); err != nil {
			panic("failed to load Chinese translations: " + err.Error())
		}
	}
}

// Localizer wraps go-i18n localizer with convenience methods
type Localizer struct {
	localizer *i18n.Localizer
}

// NewLocalizer creates a new localizer for the given locale
// Supported locales: "en" (English), "zh" (Chinese Simplified)
func NewLocalizer(locale string) *Localizer {
	// Normalize locale string
	var lang language.Tag
	switch locale {
	case "zh", "zh-CN", "zh_CN":
		lang = language.Chinese
	case "en", "en-US", "en_US":
		lang = language.English
	default:
		// Default to English for unknown locales
		lang = language.English
	}

	return &Localizer{
		localizer: i18n.NewLocalizer(bundle, lang.String()),
	}
}

// T translates a message ID to the localized string
func (l *Localizer) T(messageID string) string {
	msg, err := l.localizer.Localize(&i18n.LocalizeConfig{
		MessageID: messageID,
	})
	if err != nil {
		// Return message ID if translation not found
		return messageID
	}
	return msg
}

// TP translates a message with plural support
// count is used to determine plural form
func (l *Localizer) TP(messageID string, count int) string {
	msg, err := l.localizer.Localize(&i18n.LocalizeConfig{
		MessageID:   messageID,
		PluralCount: count,
	})
	if err != nil {
		return messageID
	}
	return msg
}

// TF translates a message with template data
// templateData contains variables to be substituted in the translated string
func (l *Localizer) TF(messageID string, templateData map[string]interface{}) string {
	msg, err := l.localizer.Localize(&i18n.LocalizeConfig{
		MessageID:    messageID,
		TemplateData: templateData,
	})
	if err != nil {
		// Try to return a useful debug string
		dataJSON, _ := json.Marshal(templateData)
		return messageID + " " + string(dataJSON)
	}
	return msg
}

// TWithDefault translates a message with a default fallback
func (l *Localizer) TWithDefault(messageID, defaultMsg string) string {
	msg, err := l.localizer.Localize(&i18n.LocalizeConfig{
		DefaultMessage: &i18n.Message{
			ID:    messageID,
			Other: defaultMsg,
		},
	})
	if err != nil {
		return defaultMsg
	}
	return msg
}
