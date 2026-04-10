package slack

import (
	"testing"

	slacklib "github.com/slack-go/slack"
	"github.com/stahnma/mandatoryFun/zoom-notifier/internal/store"
)

func TestBuildSetSuffixModal(t *testing.T) {
	filters := []*store.MeetingFilter{
		{ID: 1, TenantID: "T1", Pattern: "Standup"},
		{ID: 2, TenantID: "T1", Pattern: "Retro"},
	}

	view := BuildSetSuffixModal("current suffix", filters)

	if view.CallbackID != "set_suffix" {
		t.Errorf("expected callback_id set_suffix, got %s", view.CallbackID)
	}
	if view.Type != slacklib.VTModal {
		t.Errorf("expected type modal, got %s", view.Type)
	}
	if len(view.Blocks.BlockSet) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(view.Blocks.BlockSet))
	}

	// Check filter block
	filterBlock, ok := view.Blocks.BlockSet[0].(*slacklib.InputBlock)
	if !ok {
		t.Fatal("first block is not an InputBlock")
	}
	if filterBlock.BlockID != "filter_block" {
		t.Errorf("expected block_id filter_block, got %s", filterBlock.BlockID)
	}
	selectElem, ok := filterBlock.Element.(*slacklib.SelectBlockElement)
	if !ok {
		t.Fatal("filter element is not a SelectBlockElement")
	}
	if selectElem.ActionID != "filter_select" {
		t.Errorf("expected action_id filter_select, got %s", selectElem.ActionID)
	}
	// 1 default + 2 filters = 3 options
	if len(selectElem.Options) != 3 {
		t.Errorf("expected 3 options, got %d", len(selectElem.Options))
	}

	// Check suffix block
	suffixBlock, ok := view.Blocks.BlockSet[1].(*slacklib.InputBlock)
	if !ok {
		t.Fatal("second block is not an InputBlock")
	}
	if suffixBlock.BlockID != "suffix_block" {
		t.Errorf("expected block_id suffix_block, got %s", suffixBlock.BlockID)
	}
	inputElem, ok := suffixBlock.Element.(*slacklib.PlainTextInputBlockElement)
	if !ok {
		t.Fatal("suffix element is not a PlainTextInputBlockElement")
	}
	if inputElem.ActionID != "suffix_input" {
		t.Errorf("expected action_id suffix_input, got %s", inputElem.ActionID)
	}
	if inputElem.InitialValue != "current suffix" {
		t.Errorf("expected initial value 'current suffix', got '%s'", inputElem.InitialValue)
	}
}

func TestBuildSetSuffixModal_EmptySuffix(t *testing.T) {
	view := BuildSetSuffixModal("", nil)

	if view.CallbackID != "set_suffix" {
		t.Errorf("expected callback_id set_suffix, got %s", view.CallbackID)
	}

	suffixBlock := view.Blocks.BlockSet[1].(*slacklib.InputBlock)
	inputElem := suffixBlock.Element.(*slacklib.PlainTextInputBlockElement)
	if inputElem.InitialValue != "" {
		t.Errorf("expected empty initial value, got '%s'", inputElem.InitialValue)
	}
}

func TestBuildSetLinkModal(t *testing.T) {
	filters := []*store.MeetingFilter{
		{ID: 1, TenantID: "T1", Pattern: "Standup"},
	}

	view := BuildSetLinkModal(true, filters)

	if view.CallbackID != "set_link" {
		t.Errorf("expected callback_id set_link, got %s", view.CallbackID)
	}
	if view.Type != slacklib.VTModal {
		t.Errorf("expected type modal, got %s", view.Type)
	}
	if len(view.Blocks.BlockSet) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(view.Blocks.BlockSet))
	}

	// Check filter block
	filterBlock := view.Blocks.BlockSet[0].(*slacklib.InputBlock)
	if filterBlock.BlockID != "filter_block" {
		t.Errorf("expected block_id filter_block, got %s", filterBlock.BlockID)
	}
	selectElem := filterBlock.Element.(*slacklib.SelectBlockElement)
	if selectElem.ActionID != "filter_select" {
		t.Errorf("expected action_id filter_select, got %s", selectElem.ActionID)
	}

	// Check link block
	linkBlock := view.Blocks.BlockSet[1].(*slacklib.InputBlock)
	if linkBlock.BlockID != "link_block" {
		t.Errorf("expected block_id link_block, got %s", linkBlock.BlockID)
	}
	radioElem, ok := linkBlock.Element.(*slacklib.RadioButtonsBlockElement)
	if !ok {
		t.Fatal("link element is not a RadioButtonsBlockElement")
	}
	if radioElem.ActionID != "link_input" {
		t.Errorf("expected action_id link_input, got %s", radioElem.ActionID)
	}
	if len(radioElem.Options) != 2 {
		t.Errorf("expected 2 options (On/Off), got %d", len(radioElem.Options))
	}
	// currentLink=true so initial should be "on"
	if radioElem.InitialOption == nil || radioElem.InitialOption.Value != "on" {
		t.Error("expected initial option to be 'on'")
	}
}

func TestBuildSetLinkModal_Off(t *testing.T) {
	view := BuildSetLinkModal(false, nil)

	linkBlock := view.Blocks.BlockSet[1].(*slacklib.InputBlock)
	radioElem := linkBlock.Element.(*slacklib.RadioButtonsBlockElement)
	if radioElem.InitialOption == nil || radioElem.InitialOption.Value != "off" {
		t.Error("expected initial option to be 'off'")
	}
}

func TestBuildSubscribeModal(t *testing.T) {
	view := BuildSubscribeModal()

	if view.CallbackID != "subscribe" {
		t.Errorf("expected callback_id subscribe, got %s", view.CallbackID)
	}
	if view.Type != slacklib.VTModal {
		t.Errorf("expected type modal, got %s", view.Type)
	}
	if len(view.Blocks.BlockSet) != 1 {
		t.Fatalf("expected 1 block, got %d", len(view.Blocks.BlockSet))
	}

	channelBlock := view.Blocks.BlockSet[0].(*slacklib.InputBlock)
	if channelBlock.BlockID != "channel_block" {
		t.Errorf("expected block_id channel_block, got %s", channelBlock.BlockID)
	}
	selectElem, ok := channelBlock.Element.(*slacklib.SelectBlockElement)
	if !ok {
		t.Fatal("channel element is not a SelectBlockElement")
	}
	if selectElem.ActionID != "channel_select" {
		t.Errorf("expected action_id channel_select, got %s", selectElem.ActionID)
	}
	if selectElem.Type != slacklib.OptTypeConversations {
		t.Errorf("expected type conversations_select, got %s", selectElem.Type)
	}
}

func TestBuildFilterModal(t *testing.T) {
	view := BuildFilterModal()

	if view.CallbackID != "add_filter" {
		t.Errorf("expected callback_id add_filter, got %s", view.CallbackID)
	}
	if view.Type != slacklib.VTModal {
		t.Errorf("expected type modal, got %s", view.Type)
	}
	if len(view.Blocks.BlockSet) != 3 {
		t.Fatalf("expected 3 blocks, got %d", len(view.Blocks.BlockSet))
	}

	// Check pattern block
	patternBlock := view.Blocks.BlockSet[0].(*slacklib.InputBlock)
	if patternBlock.BlockID != "pattern_block" {
		t.Errorf("expected block_id pattern_block, got %s", patternBlock.BlockID)
	}
	patternInput, ok := patternBlock.Element.(*slacklib.PlainTextInputBlockElement)
	if !ok {
		t.Fatal("pattern element is not a PlainTextInputBlockElement")
	}
	if patternInput.ActionID != "pattern_input" {
		t.Errorf("expected action_id pattern_input, got %s", patternInput.ActionID)
	}

	// Check suffix block (optional)
	suffixBlock := view.Blocks.BlockSet[1].(*slacklib.InputBlock)
	if suffixBlock.BlockID != "suffix_block" {
		t.Errorf("expected block_id suffix_block, got %s", suffixBlock.BlockID)
	}
	if !suffixBlock.Optional {
		t.Error("expected suffix block to be optional")
	}
	suffixInput, ok := suffixBlock.Element.(*slacklib.PlainTextInputBlockElement)
	if !ok {
		t.Fatal("suffix element is not a PlainTextInputBlockElement")
	}
	if suffixInput.ActionID != "suffix_input" {
		t.Errorf("expected action_id suffix_input, got %s", suffixInput.ActionID)
	}

	// Check link block (optional)
	linkBlock := view.Blocks.BlockSet[2].(*slacklib.InputBlock)
	if linkBlock.BlockID != "link_block" {
		t.Errorf("expected block_id link_block, got %s", linkBlock.BlockID)
	}
	if !linkBlock.Optional {
		t.Error("expected link block to be optional")
	}
	linkRadio, ok := linkBlock.Element.(*slacklib.RadioButtonsBlockElement)
	if !ok {
		t.Fatal("link element is not a RadioButtonsBlockElement")
	}
	if linkRadio.ActionID != "link_input" {
		t.Errorf("expected action_id link_input, got %s", linkRadio.ActionID)
	}
	if len(linkRadio.Options) != 3 {
		t.Errorf("expected 3 options (On/Off/Use default), got %d", len(linkRadio.Options))
	}
	if linkRadio.InitialOption == nil || linkRadio.InitialOption.Value != "default" {
		t.Error("expected initial option to be 'default'")
	}
}
