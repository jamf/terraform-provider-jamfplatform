// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package components

import (
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework-validators/boolvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// MathSettingsComponent represents a strongly-typed math settings component.
type MathSettingsComponent struct {
	CalculatorBasicModeAddSquareRoot   types.Bool `tfsdk:"calculator_basic_mode_add_square_root"`
	CalculatorScientificModeEnabled    types.Bool `tfsdk:"calculator_scientific_mode_enabled"`
	CalculatorProgrammerModeEnabled    types.Bool `tfsdk:"calculator_programmer_mode_enabled"`
	CalculatorMathNotesModeEnabled     types.Bool `tfsdk:"calculator_math_notes_mode_enabled"`
	CalculatorInputModesUnitConversion types.Bool `tfsdk:"calculator_input_modes_unit_conversion"`
	CalculatorInputModesRPN            types.Bool `tfsdk:"calculator_input_modes_rpn"`
	SystemBehaviorKeyboardSuggestions  types.Bool `tfsdk:"system_behavior_keyboard_suggestions"`
	SystemBehaviorMathNotes            types.Bool `tfsdk:"system_behavior_math_notes"`
}

// GetIdentifier returns the component identifier for math settings.
func (c *MathSettingsComponent) GetIdentifier() string {
	return "com.jamf.ddm.math-settings"
}

// MathSettingsComponentSchema returns the Terraform schema for math settings component.
func MathSettingsComponentSchema() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"calculator_basic_mode_add_square_root": schema.BoolAttribute{
			MarkdownDescription: "Add the square root button to the basic calculator by replacing the +/- button.",
			Optional:            true,
		},
		"calculator_scientific_mode_enabled": schema.BoolAttribute{
			MarkdownDescription: "Controls whether the scientific mode is enabled in Calculator.",
			Optional:            true,
		},
		"calculator_programmer_mode_enabled": schema.BoolAttribute{
			MarkdownDescription: "Controls whether the programmer mode is enabled in Calculator.",
			Optional:            true,
		},
		"calculator_math_notes_mode_enabled": schema.BoolAttribute{
			MarkdownDescription: "Controls whether the Math Notes mode is enabled in Calculator.",
			Optional:            true,
		},
		"calculator_input_modes_unit_conversion": schema.BoolAttribute{
			MarkdownDescription: "Configures whether unit conversions are enabled in Calculator. Also requires `calculator_input_modes_rpn` to be set.",
			Optional:            true,
			Validators: []validator.Bool{
				boolvalidator.AlsoRequires(path.MatchRelative().AtParent().AtName("calculator_input_modes_rpn")),
			},
		},
		"calculator_input_modes_rpn": schema.BoolAttribute{
			MarkdownDescription: "Configures whether RPN input is enabled in Calculator. Also requires `calculator_input_modes_unit_conversion` to be set.",
			Optional:            true,
			Validators: []validator.Bool{
				boolvalidator.AlsoRequires(path.MatchRelative().AtParent().AtName("calculator_input_modes_unit_conversion")),
			},
		},
		"system_behavior_keyboard_suggestions": schema.BoolAttribute{
			MarkdownDescription: "Controls whether keyboard suggestions include math solutions. Also requires `system_behavior_math_notes` to be set.",
			Optional:            true,
			Validators: []validator.Bool{
				boolvalidator.AlsoRequires(path.MatchRelative().AtParent().AtName("system_behavior_math_notes")),
			},
		},
		"system_behavior_math_notes": schema.BoolAttribute{
			MarkdownDescription: "Controls whether Math Notes is allowed in other apps such as Notes. Also requires `system_behavior_keyboard_suggestions` to be set.",
			Optional:            true,
			Validators: []validator.Bool{
				boolvalidator.AlsoRequires(path.MatchRelative().AtParent().AtName("system_behavior_keyboard_suggestions")),
			},
		},
	}
}

// ToRawConfiguration converts the typed component to raw JSON configuration.
func (c *MathSettingsComponent) ToRawConfiguration() (json.RawMessage, error) {
	cfg := blueprints.MathSettingsConfiguration{}

	calc := &blueprints.Calculator{}

	calc.BasicMode = &blueprints.BasicMode{
		Included:      new(helpers.IsConfiguredValue(c.CalculatorBasicModeAddSquareRoot)),
		AddSquareRoot: c.CalculatorBasicModeAddSquareRoot.ValueBool(),
	}
	if !helpers.IsConfiguredValue(c.CalculatorBasicModeAddSquareRoot) {
		calc.BasicMode.AddSquareRoot = true
	}

	calc.ScientificMode = &blueprints.ScientificMode{
		Included: new(helpers.IsConfiguredValue(c.CalculatorScientificModeEnabled)),
		Enabled:  c.CalculatorScientificModeEnabled.ValueBool(),
	}
	if !helpers.IsConfiguredValue(c.CalculatorScientificModeEnabled) {
		calc.ScientificMode.Enabled = true
	}

	calc.ProgrammerMode = &blueprints.ProgrammerMode{
		Included: new(helpers.IsConfiguredValue(c.CalculatorProgrammerModeEnabled)),
		Enabled:  c.CalculatorProgrammerModeEnabled.ValueBool(),
	}
	if !helpers.IsConfiguredValue(c.CalculatorProgrammerModeEnabled) {
		calc.ProgrammerMode.Enabled = true
	}

	calc.MathNotesMode = &blueprints.MathNotesMode{
		Included: new(helpers.IsConfiguredValue(c.CalculatorMathNotesModeEnabled)),
		Enabled:  c.CalculatorMathNotesModeEnabled.ValueBool(),
	}
	if !helpers.IsConfiguredValue(c.CalculatorMathNotesModeEnabled) {
		calc.MathNotesMode.Enabled = true
	}

	hasInputModes := helpers.IsConfiguredValue(c.CalculatorInputModesUnitConversion) ||
		helpers.IsConfiguredValue(c.CalculatorInputModesRPN)

	unitConversion := true
	rpn := true
	if helpers.IsConfiguredValue(c.CalculatorInputModesUnitConversion) {
		unitConversion = c.CalculatorInputModesUnitConversion.ValueBool()
	}
	if helpers.IsConfiguredValue(c.CalculatorInputModesRPN) {
		rpn = c.CalculatorInputModesRPN.ValueBool()
	}
	calc.InputModes = &blueprints.InputModes{
		Included:       new(hasInputModes),
		UnitConversion: unitConversion,
		RPN:            rpn,
	}

	cfg.Calculator = calc

	hasSystemBehavior := helpers.IsConfiguredValue(c.SystemBehaviorKeyboardSuggestions) ||
		helpers.IsConfiguredValue(c.SystemBehaviorMathNotes)

	keyboardSuggestions := true
	mathNotes := true
	if helpers.IsConfiguredValue(c.SystemBehaviorKeyboardSuggestions) {
		keyboardSuggestions = c.SystemBehaviorKeyboardSuggestions.ValueBool()
	}
	if helpers.IsConfiguredValue(c.SystemBehaviorMathNotes) {
		mathNotes = c.SystemBehaviorMathNotes.ValueBool()
	}
	cfg.SystemBehavior = &blueprints.SystemBehavior{
		Included:            new(hasSystemBehavior),
		KeyboardSuggestions: keyboardSuggestions,
		MathNotes:           mathNotes,
	}

	return json.Marshal(cfg)
}

// FromRawConfiguration populates the typed component from raw JSON configuration.
func (c *MathSettingsComponent) FromRawConfiguration(raw json.RawMessage) error {
	var cfg blueprints.MathSettingsConfiguration
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return err
	}

	c.CalculatorBasicModeAddSquareRoot = types.BoolNull()
	c.CalculatorScientificModeEnabled = types.BoolNull()
	c.CalculatorProgrammerModeEnabled = types.BoolNull()
	c.CalculatorMathNotesModeEnabled = types.BoolNull()
	c.CalculatorInputModesUnitConversion = types.BoolNull()
	c.CalculatorInputModesRPN = types.BoolNull()
	c.SystemBehaviorKeyboardSuggestions = types.BoolNull()
	c.SystemBehaviorMathNotes = types.BoolNull()

	if calc := cfg.Calculator; calc != nil {
		if bm := calc.BasicMode; bm != nil && bm.Included != nil && *bm.Included {
			c.CalculatorBasicModeAddSquareRoot = types.BoolValue(bm.AddSquareRoot)
		}
		if sm := calc.ScientificMode; sm != nil && sm.Included != nil && *sm.Included {
			c.CalculatorScientificModeEnabled = types.BoolValue(sm.Enabled)
		}
		if pm := calc.ProgrammerMode; pm != nil && pm.Included != nil && *pm.Included {
			c.CalculatorProgrammerModeEnabled = types.BoolValue(pm.Enabled)
		}
		if mm := calc.MathNotesMode; mm != nil && mm.Included != nil && *mm.Included {
			c.CalculatorMathNotesModeEnabled = types.BoolValue(mm.Enabled)
		}
		if im := calc.InputModes; im != nil && im.Included != nil && *im.Included {
			c.CalculatorInputModesUnitConversion = types.BoolValue(im.UnitConversion)
			c.CalculatorInputModesRPN = types.BoolValue(im.RPN)
		}
	}

	if sb := cfg.SystemBehavior; sb != nil && sb.Included != nil && *sb.Included {
		c.SystemBehaviorKeyboardSuggestions = types.BoolValue(sb.KeyboardSuggestions)
		c.SystemBehaviorMathNotes = types.BoolValue(sb.MathNotes)
	}

	return nil
}

// ToClientComponent converts the typed component to the format expected by the Blueprint API client.
func (c *MathSettingsComponent) ToClientComponent() (*blueprints.Component, error) {
	cfg, err := c.ToRawConfiguration()
	if err != nil {
		return nil, err
	}
	return &blueprints.Component{
		Identifier:    c.GetIdentifier(),
		Configuration: cfg,
	}, nil
}
