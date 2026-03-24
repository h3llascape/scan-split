package ocr

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
)

// mockStudents is a fixed list of realistic test students for UI and pipeline development.
var mockStudents = []struct {
	name  string
	group string
}{
	{"Иванов Иван Иванович", "РИ-330942"},
	{"Петрова Мария Сергеевна", "РИ-330943"},
	{"Сидоров Алексей Владимирович", "РИ-330944"},
	{"Козлова Анна Николаевна", "МО-230115"},
	{"Новиков Дмитрий Андреевич", "МО-230116"},
	{"Морозова Екатерина Игоревна", "ЭП-430051"},
	{"Волков Сергей Петрович", "ЭП-430052"},
	{"Лебедева Ольга Михайловна", "БИ-130871"},
	{"Федоров Никита Александрович", "БИ-130872"},
	{"Соколова Виктория Дмитриевна", "МА-531001"},
}

// MockProvider is a development OCR provider that returns synthetic student data.
// Every 10th call produces an unreadable page to exercise the orphan path.
type MockProvider struct {
	logger  *slog.Logger
	counter atomic.Int64
}

// NewMockProvider creates a new MockProvider.
func NewMockProvider(logger *slog.Logger) *MockProvider {
	return &MockProvider{logger: logger}
}

// RecognizeText returns a fake OCR result without reading imageData.
func (m *MockProvider) RecognizeText(_ context.Context, _ []byte) (string, error) {
	n := m.counter.Add(1)

	// Simulate ~10% unreadable pages to exercise orphan handling.
	if n%10 == 0 {
		m.logger.Debug("mock ocr: unreadable page")
		return "нечитаемый документ xxxxx", nil
	}

	s := mockStudents[(n-1)%int64(len(mockStudents))]
	m.logger.Debug("mock ocr: returning student", "name", s.name, "group", s.group)

	return fmt.Sprintf(
		"МИНИСТЕРСТВО НАУКИ И ВЫСШЕГО ОБРАЗОВАНИЯ\n"+
			"Федеральное государственное бюджетное образовательное учреждение\n\n"+
			"ОТЧЁТ ПО ПРОИЗВОДСТВЕННОЙ ПРАКТИКЕ\n\n"+
			"Студент: %s\n"+
			"Группа: %s\n"+
			"Направление подготовки: 09.03.03\n\n"+
			"Руководитель практики от предприятия:\n"+
			"___________________\n",
		s.name, s.group,
	), nil
}
