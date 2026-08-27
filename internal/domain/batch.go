package domain

import "strings"

type BatchUnitInput struct {
	Row  int
	Unit StratigraphicUnit
}

type RowValidationError struct {
	Row     int    `json:"row"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

func ValidateUnitBatch(status DossierStatus, existing []StratigraphicUnit, rows []BatchUnitInput) []RowValidationError {
	errors := []RowValidationError{}
	if status != StatusDraft && status != StatusRemediate && status != StatusReturned {
		return []RowValidationError{{Row: 0, Field: "status", Message: "只有草拟、整改中或已退回案卷可以批量登记单位"}}
	}
	existingCodes := map[string]bool{}
	for _, unit := range existing {
		existingCodes[unit.UnitCode] = true
	}
	seen := map[string]int{}
	for _, input := range rows {
		unit := input.Unit
		add := func(field, message string) {
			errors = append(errors, RowValidationError{Row: input.Row, Field: field, Message: message})
		}
		if strings.TrimSpace(unit.UnitCode) == "" {
			add("unitCode", "地层单位编号不能为空")
		}
		if strings.TrimSpace(unit.UnitType) == "" {
			add("unitType", "地层单位类型不能为空")
		}
		if strings.TrimSpace(unit.Description) == "" {
			add("description", "地层单位描述不能为空")
		}
		if unit.TopElevation < unit.BottomElevation {
			add("topElevation", "界面顶标高不能低于底标高")
		}
		if strings.TrimSpace(unit.SoilTraits) == "" {
			add("soilTraits", "土质特征不能为空")
		}
		if len(unit.PhotoRefs) == 0 {
			add("photoRefs", "至少需要一条照片引用")
		}
		if first, ok := seen[unit.UnitCode]; ok && unit.UnitCode != "" {
			add("unitCode", "与批次第 "+itoa(first)+" 行编号重复")
		} else {
			seen[unit.UnitCode] = input.Row
		}
		if existingCodes[unit.UnitCode] && unit.UnitCode != "" {
			add("unitCode", "单位编号已存在")
		}
	}
	return errors
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := [20]byte{}
	i := len(digits)
	for value > 0 {
		i--
		digits[i] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[i:])
}
