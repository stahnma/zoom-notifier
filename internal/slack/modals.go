package slack

import (
	"fmt"

	slacklib "github.com/slack-go/slack"
	"github.com/stahnma/mandatoryFun/zoom-notifier/internal/store"
)

// ModalOpener opens a Slack modal view using a trigger ID.
type ModalOpener interface {
	OpenView(triggerID string, view slacklib.ModalViewRequest) (*slacklib.ViewResponse, error)
}

// SlackModalOpener implements ModalOpener using the slack-go client.
type SlackModalOpener struct {
	client *slacklib.Client
}

// NewSlackModalOpener creates a ModalOpener that uses the given bot token.
func NewSlackModalOpener(botToken string) *SlackModalOpener {
	return &SlackModalOpener{
		client: slacklib.New(botToken),
	}
}

// OpenView opens a modal view in Slack.
func (o *SlackModalOpener) OpenView(triggerID string, view slacklib.ModalViewRequest) (*slacklib.ViewResponse, error) {
	return o.client.OpenView(triggerID, view)
}

// buildFilterOptions creates the "Apply to" options list from filters.
func buildFilterOptions(filters []*store.MeetingFilter) []*slacklib.OptionBlockObject {
	options := []*slacklib.OptionBlockObject{
		slacklib.NewOptionBlockObject(
			"all",
			slacklib.NewTextBlockObject("plain_text", "All meetings (default)", false, false),
			nil,
		),
	}
	for _, f := range filters {
		options = append(options, slacklib.NewOptionBlockObject(
			fmt.Sprintf("filter_%d", f.ID),
			slacklib.NewTextBlockObject("plain_text", f.Pattern, false, false),
			nil,
		))
	}
	return options
}

// BuildSetSuffixModal creates a modal for setting the message suffix.
func BuildSetSuffixModal(currentSuffix string, filters []*store.MeetingFilter) slacklib.ModalViewRequest {
	filterOptions := buildFilterOptions(filters)

	filterSelect := slacklib.NewOptionsSelectBlockElement(
		slacklib.OptTypeStatic,
		slacklib.NewTextBlockObject("plain_text", "Select scope", false, false),
		"filter_select",
		filterOptions...,
	)
	filterSelect.InitialOption = filterOptions[0]

	filterBlock := slacklib.NewInputBlock(
		"filter_block",
		slacklib.NewTextBlockObject("plain_text", "Apply to", false, false),
		nil,
		filterSelect,
	)

	suffixInput := slacklib.NewPlainTextInputBlockElement(
		slacklib.NewTextBlockObject("plain_text", "Enter suffix text", false, false),
		"suffix_input",
	)
	if currentSuffix != "" {
		suffixInput = suffixInput.WithInitialValue(currentSuffix)
	}

	suffixBlock := slacklib.NewInputBlock(
		"suffix_block",
		slacklib.NewTextBlockObject("plain_text", "Message suffix", false, false),
		nil,
		suffixInput,
	)

	return slacklib.ModalViewRequest{
		Type:       slacklib.VTModal,
		CallbackID: "set_suffix",
		Title:      slacklib.NewTextBlockObject("plain_text", "Set Message Suffix", false, false),
		Submit:     slacklib.NewTextBlockObject("plain_text", "Save", false, false),
		Close:      slacklib.NewTextBlockObject("plain_text", "Cancel", false, false),
		Blocks: slacklib.Blocks{
			BlockSet: []slacklib.Block{filterBlock, suffixBlock},
		},
	}
}

// BuildSetLinkModal creates a modal for toggling meeting link inclusion.
func BuildSetLinkModal(currentLink bool, filters []*store.MeetingFilter) slacklib.ModalViewRequest {
	filterOptions := buildFilterOptions(filters)

	filterSelect := slacklib.NewOptionsSelectBlockElement(
		slacklib.OptTypeStatic,
		slacklib.NewTextBlockObject("plain_text", "Select scope", false, false),
		"filter_select",
		filterOptions...,
	)
	filterSelect.InitialOption = filterOptions[0]

	filterBlock := slacklib.NewInputBlock(
		"filter_block",
		slacklib.NewTextBlockObject("plain_text", "Apply to", false, false),
		nil,
		filterSelect,
	)

	onOption := slacklib.NewOptionBlockObject(
		"on",
		slacklib.NewTextBlockObject("plain_text", "On", false, false),
		nil,
	)
	offOption := slacklib.NewOptionBlockObject(
		"off",
		slacklib.NewTextBlockObject("plain_text", "Off", false, false),
		nil,
	)

	linkRadio := slacklib.NewRadioButtonsBlockElement("link_input", onOption, offOption)
	if currentLink {
		linkRadio.InitialOption = onOption
	} else {
		linkRadio.InitialOption = offOption
	}

	linkBlock := slacklib.NewInputBlock(
		"link_block",
		slacklib.NewTextBlockObject("plain_text", "Include meeting link", false, false),
		nil,
		linkRadio,
	)

	return slacklib.ModalViewRequest{
		Type:       slacklib.VTModal,
		CallbackID: "set_link",
		Title:      slacklib.NewTextBlockObject("plain_text", "Set Meeting Link", false, false),
		Submit:     slacklib.NewTextBlockObject("plain_text", "Save", false, false),
		Close:      slacklib.NewTextBlockObject("plain_text", "Cancel", false, false),
		Blocks: slacklib.Blocks{
			BlockSet: []slacklib.Block{filterBlock, linkBlock},
		},
	}
}

// BuildSubscribeModal creates a modal for subscribing a channel.
func BuildSubscribeModal() slacklib.ModalViewRequest {
	channelSelect := slacklib.NewOptionsSelectBlockElement(
		slacklib.OptTypeConversations,
		slacklib.NewTextBlockObject("plain_text", "Select a channel", false, false),
		"channel_select",
	)
	channelSelect.DefaultToCurrentConversation = true

	channelBlock := slacklib.NewInputBlock(
		"channel_block",
		slacklib.NewTextBlockObject("plain_text", "Channel", false, false),
		nil,
		channelSelect,
	)

	return slacklib.ModalViewRequest{
		Type:       slacklib.VTModal,
		CallbackID: "subscribe",
		Title:      slacklib.NewTextBlockObject("plain_text", "Subscribe Channel", false, false),
		Submit:     slacklib.NewTextBlockObject("plain_text", "Subscribe", false, false),
		Close:      slacklib.NewTextBlockObject("plain_text", "Cancel", false, false),
		Blocks: slacklib.Blocks{
			BlockSet: []slacklib.Block{channelBlock},
		},
	}
}

// BuildFilterModal creates a modal for adding a meeting filter.
func BuildFilterModal() slacklib.ModalViewRequest {
	patternInput := slacklib.NewPlainTextInputBlockElement(
		slacklib.NewTextBlockObject("plain_text", "e.g. Daily Standup", false, false),
		"pattern_input",
	)

	patternBlock := slacklib.NewInputBlock(
		"pattern_block",
		slacklib.NewTextBlockObject("plain_text", "Filter pattern", false, false),
		nil,
		patternInput,
	)

	suffixInput := slacklib.NewPlainTextInputBlockElement(
		slacklib.NewTextBlockObject("plain_text", "Optional suffix override", false, false),
		"suffix_input",
	)

	suffixBlock := slacklib.NewInputBlock(
		"suffix_block",
		slacklib.NewTextBlockObject("plain_text", "Message suffix", false, false),
		nil,
		suffixInput,
	)
	suffixBlock.Optional = true

	onOption := slacklib.NewOptionBlockObject(
		"on",
		slacklib.NewTextBlockObject("plain_text", "On", false, false),
		nil,
	)
	offOption := slacklib.NewOptionBlockObject(
		"off",
		slacklib.NewTextBlockObject("plain_text", "Off", false, false),
		nil,
	)
	defaultOption := slacklib.NewOptionBlockObject(
		"default",
		slacklib.NewTextBlockObject("plain_text", "Use default", false, false),
		nil,
	)

	linkRadio := slacklib.NewRadioButtonsBlockElement("link_input", onOption, offOption, defaultOption)
	linkRadio.InitialOption = defaultOption

	linkBlock := slacklib.NewInputBlock(
		"link_block",
		slacklib.NewTextBlockObject("plain_text", "Include meeting link", false, false),
		nil,
		linkRadio,
	)
	linkBlock.Optional = true

	return slacklib.ModalViewRequest{
		Type:       slacklib.VTModal,
		CallbackID: "add_filter",
		Title:      slacklib.NewTextBlockObject("plain_text", "Add Meeting Filter", false, false),
		Submit:     slacklib.NewTextBlockObject("plain_text", "Add Filter", false, false),
		Close:      slacklib.NewTextBlockObject("plain_text", "Cancel", false, false),
		Blocks: slacklib.Blocks{
			BlockSet: []slacklib.Block{patternBlock, suffixBlock, linkBlock},
		},
	}
}
