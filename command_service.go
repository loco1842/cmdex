package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// CommandService handles all command and category CRUD operations.
type CommandService struct{}

func (s *CommandService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	return nil
}

// ========== Category Operations ==========

// GetCategories returns all categories from the DB.
func (s *CommandService) GetCategories() []Category {
	cats, err := db.GetCategories()
	if err != nil {
		fmt.Println("Error getting categories:", err)
		return []Category{}
	}
	return cats
}

// CreateCategory creates a new Category with the given name, icon, color and returns the created Category or an error.
func (s *CommandService) CreateCategory(name string, icon string, color string) (Category, error) {
	cat := Category{
		ID:        uuid.New().String(),
		Name:      name,
		Icon:      icon,
		Color:     color,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := db.CreateCategory(cat); err != nil {
		return Category{}, err
	}
	return cat, nil
}

// UpdateCategory updates a Category's name, icon, and color by id and returns the updated Category or an error.
func (s *CommandService) UpdateCategory(id string, name string, icon string, color string) (Category, error) {
	cat := Category{
		ID:        id,
		Name:      name,
		Icon:      icon,
		Color:     color,
		UpdatedAt: time.Now(),
	}
	if err := db.UpdateCategory(cat); err != nil {
		return Category{}, err
	}
	cats, _ := db.GetCategories()
	for _, c := range cats {
		if c.ID == id {
			return c, nil
		}
	}
	return cat, nil
}

// DeleteCategory deletes the category with the given id.
func (s *CommandService) DeleteCategory(id string) error {
	return db.DeleteCategory(id)
}

// ========== Command Operations ==========

// GetCommands returns all commands from the DB.
func (s *CommandService) GetCommands() []Command {
	cmds, err := db.GetCommands()
	if err != nil {
		fmt.Println("Error getting commands:", err)
		return []Command{}
	}
	return cmds
}

// GetCommandsByCategory returns all commands belonging to the given categoryID.
func (s *CommandService) GetCommandsByCategory(categoryID string) []Command {
	cmds, err := db.GetCommandsByCategory(categoryID)
	if err != nil {
		fmt.Println("Error getting commands:", err)
		return []Command{}
	}
	return cmds
}

// CreateCommand creates a new Command with the given fields and returns the created Command or an error.
func (s *CommandService) CreateCommand(
	title, description, scriptBody, categoryID string,
	tags []string,
	variables []VariableDefinition,
	workingDir OSPathMap,
) (Command, error) {
	if tags == nil {
		tags = []string{}
	}
	if variables == nil {
		variables = []VariableDefinition{}
	}

	for i := range variables {
		variables[i].SortOrder = i
	}

	scriptContent := GenerateScript(scriptBody)

	cmd := Command{
		ID:            uuid.New().String(),
		Title:         sql.NullString{String: title, Valid: title != ""},
		Description:   sql.NullString{String: description, Valid: description != ""},
		ScriptContent: scriptContent,
		CategoryID:    categoryID,
		Tags:          tags,
		Variables:     variables,
		Presets:       []VariablePreset{},
		WorkingDir:    workingDir,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	if err := db.CreateCommand(cmd); err != nil {
		return Command{}, err
	}
	return cmd, nil
}

// UpdateCommand updates a Command's fields by id and returns the updated Command or an error.
func (s *CommandService) UpdateCommand(
	id, title, description, scriptBody, categoryID string,
	tags []string,
	variables []VariableDefinition,
	workingDir OSPathMap,
) (Command, error) {
	if tags == nil {
		tags = []string{}
	}
	if variables == nil {
		variables = []VariableDefinition{}
	}

	for i := range variables {
		variables[i].SortOrder = i
	}

	scriptContent := GenerateScript(scriptBody)

	cmd := Command{
		ID:            id,
		Title:         sql.NullString{String: title, Valid: title != ""},
		Description:   sql.NullString{String: description, Valid: description != ""},
		ScriptContent: scriptContent,
		CategoryID:    categoryID,
		Tags:          tags,
		Variables:     variables,
		WorkingDir:    workingDir,
		UpdatedAt:     time.Now(),
	}

	if err := db.UpdateCommand(cmd); err != nil {
		return Command{}, err
	}

	return db.GetCommand(id)
}

// RenameCommand sets a new title for the command with the given id and returns the updated Command.
func (s *CommandService) RenameCommand(id string, newTitle string) (Command, error) {
	if err := db.RenameCommand(id, newTitle); err != nil {
		return Command{}, err
	}
	return db.GetCommand(id)
}

// DeleteCommand deletes the command with the given id.
func (s *CommandService) DeleteCommand(id string) error {
	return db.DeleteCommand(id)
}

// ReorderCommand moves a command to a new position within a category (or to a
// different category). newCategoryId may be empty string for uncategorized.
// newPosition is the 0-based index within the target category.
func (s *CommandService) ReorderCommand(id string, newPosition int, newCategoryId string) ([]Command, error) {
	if err := db.UpdateCommandPosition(id, newCategoryId, newPosition); err != nil {
		return nil, fmt.Errorf("reorder command: %w", err)
	}
	return db.GetCommands()
}

// GetScriptContent returns the full script content for the editor
func (s *CommandService) GetScriptContent(commandID string) (string, error) {
	cmd, err := db.GetCommand(commandID)
	if err != nil {
		return "", err
	}
	return cmd.ScriptContent, nil
}

// GetScriptBody returns just the body (for simple mode editing)
func (s *CommandService) GetScriptBody(commandID string) (string, error) {
	cmd, err := db.GetCommand(commandID)
	if err != nil {
		return "", err
	}
	return ParseScriptBody(cmd.ScriptContent), nil
}

// ========== Search ==========

// SearchCommands returns commands matching the given query string.
func (s *CommandService) SearchCommands(query string) []Command {
	if query == "" {
		return s.GetCommands()
	}

	cmds, err := db.SearchCommands(query)
	if err != nil {
		fmt.Println("Error searching commands:", err)
		return []Command{}
	}
	return cmds
}

// ========== Variable Presets ==========

// GetPresets returns all variable presets for the given commandID.
func (s *CommandService) GetPresets(commandID string) []VariablePreset {
	presets, err := db.GetPresets(commandID)
	if err != nil {
		fmt.Println("Error getting presets:", err)
		return []VariablePreset{}
	}
	return presets
}

// SavePreset creates a new VariablePreset for the given commandID and returns it or an error.
func (s *CommandService) SavePreset(commandID string, name string, values map[string]string) (VariablePreset, error) {
	preset := VariablePreset{
		ID:     uuid.New().String(),
		Name:   name,
		Values: values,
	}
	if err := db.SavePreset(commandID, preset); err != nil {
		return VariablePreset{}, err
	}
	return preset, nil
}

// UpdatePreset updates an existing VariablePreset by presetID and returns it or an error.
func (s *CommandService) UpdatePreset(
	commandID string,
	presetID string,
	name string,
	values map[string]string,
) (VariablePreset, error) {
	presets, err := db.GetPresets(commandID)
	if err != nil {
		return VariablePreset{}, fmt.Errorf("get presets for validation: %w", err)
	}
	found := false
	for _, p := range presets {
		if p.ID == presetID {
			found = true
			break
		}
	}
	if !found {
		return VariablePreset{}, fmt.Errorf("preset %s does not belong to command %s", presetID, commandID)
	}
	preset := VariablePreset{
		ID:     presetID,
		Name:   name,
		Values: values,
	}
	if err := db.UpdatePreset(preset); err != nil {
		return VariablePreset{}, err
	}
	return preset, nil
}

// DeletePreset deletes the VariablePreset with the given presetID after validating it belongs to commandID.
func (s *CommandService) DeletePreset(commandID string, presetID string) error {
	presets, err := db.GetPresets(commandID)
	if err != nil {
		return fmt.Errorf("get presets for validation: %w", err)
	}
	found := false
	for _, p := range presets {
		if p.ID == presetID {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("preset %s does not belong to command %s", presetID, commandID)
	}
	return db.DeletePreset(presetID)
}

// ReorderPresets reorders the presets for commandID to match the given presetIDs slice.
func (s *CommandService) ReorderPresets(commandID string, presetIDs []string) error {
	existing, err := db.GetPresets(commandID)
	if err != nil {
		return fmt.Errorf("get presets for validation: %w", err)
	}
	if len(presetIDs) != len(existing) {
		return fmt.Errorf("expected %d preset IDs, got %d", len(existing), len(presetIDs))
	}
	existingSet := make(map[string]bool, len(existing))
	for _, p := range existing {
		existingSet[p.ID] = true
	}
	seen := make(map[string]bool, len(presetIDs))
	for _, id := range presetIDs {
		if !existingSet[id] {
			return fmt.Errorf("preset %s does not belong to command %s", id, commandID)
		}
		if seen[id] {
			return fmt.Errorf("duplicate preset ID: %s", id)
		}
		seen[id] = true
	}
	return db.ReorderPresets(commandID, presetIDs)
}

// ========== Reset ==========

// ResetAllData deletes all data from the database.
func (s *CommandService) ResetAllData() error {
	return db.ResetAll()
}
