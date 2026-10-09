package ui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/skilled-manager/skills-manager/pkg/core"
)

// SkillListPane manages the left-side searchable skill list.
type SkillListPane struct {
	container      *fyne.Container
	searchEntry    *widget.Entry
	list           *widget.List
	allSkills      []core.Skill
	filteredSkills []core.Skill
	selectedIndex  int
	onSelect       func(skill core.Skill)
}

// NewSkillListPane creates a new searchable skill list pane.
func NewSkillListPane(onSelect func(skill core.Skill)) *SkillListPane {
	pane := &SkillListPane{
		selectedIndex:  -1,
		allSkills:      []core.Skill{},
		filteredSkills: []core.Skill{},
		onSelect:       onSelect,
	}

	pane.searchEntry = widget.NewEntry()
	pane.searchEntry.SetPlaceHolder("🔍 Search skills...")
	pane.searchEntry.OnChanged = func(query string) {
		pane.filter(query)
	}

	pane.list = widget.NewList(
		func() int {
			return len(pane.filteredSkills)
		},
		func() fyne.CanvasObject {
			title := widget.NewLabelWithStyle("Skill Name", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
			desc := widget.NewLabel("Skill description goes here...")
			desc.Wrapping = fyne.TextTruncate
			return container.NewVBox(title, desc)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id < 0 || id >= len(pane.filteredSkills) {
				return
			}
			skill := pane.filteredSkills[id]
			box := obj.(*fyne.Container)
			title := box.Objects[0].(*widget.Label)
			desc := box.Objects[1].(*widget.Label)

			title.SetText(skill.Name)
			descText := skill.Description
			if descText == "" {
				descText = "No description provided"
			}
			desc.SetText(descText)
		},
	)

	pane.list.OnSelected = func(id widget.ListItemID) {
		if id >= 0 && id < len(pane.filteredSkills) {
			pane.selectedIndex = id
			if pane.onSelect != nil {
				pane.onSelect(pane.filteredSkills[id])
			}
		}
	}

	header := container.NewVBox(
		widget.NewLabelWithStyle("Skills", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		pane.searchEntry,
	)

	pane.container = container.NewBorder(header, nil, nil, nil, pane.list)

	return pane
}

// SetSkills updates the skill list and applies current search query.
func (p *SkillListPane) SetSkills(skills []core.Skill) {
	p.allSkills = skills
	p.filter(p.searchEntry.Text)
}

// SelectSkillByName selects the skill with the matching name.
func (p *SkillListPane) SelectSkillByName(name string) {
	for i, s := range p.filteredSkills {
		if s.Name == name {
			p.list.Select(i)
			return
		}
	}
}

// GetSelectedSkill returns the currently selected skill or nil.
func (p *SkillListPane) GetSelectedSkill() *core.Skill {
	if p.selectedIndex >= 0 && p.selectedIndex < len(p.filteredSkills) {
		s := p.filteredSkills[p.selectedIndex]
		return &s
	}
	return nil
}

// Container returns the Fyne container for the skill list.
func (p *SkillListPane) Container() fyne.CanvasObject {
	return p.container
}

func (p *SkillListPane) filter(query string) {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		p.filteredSkills = make([]core.Skill, len(p.allSkills))
		copy(p.filteredSkills, p.allSkills)
	} else {
		var filtered []core.Skill
		for _, s := range p.allSkills {
			if strings.Contains(strings.ToLower(s.Name), q) || strings.Contains(strings.ToLower(s.Description), q) {
				filtered = append(filtered, s)
			}
		}
		p.filteredSkills = filtered
	}

	p.list.Refresh()

	// Maintain selection if possible
	if len(p.filteredSkills) > 0 {
		if p.selectedIndex < 0 || p.selectedIndex >= len(p.filteredSkills) {
			p.list.Select(0)
		}
	} else {
		p.selectedIndex = -1
	}
}
