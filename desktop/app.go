package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"semantic-analyzer/pkg/api"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx         context.Context
	service     *api.Service
	currentUser *api.LoginResult
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	service, err := api.NewService("../pkg/config/config.yaml")
	if err != nil {
		log.Fatalf("Ошибка инициализации сервиса: %v", err)
	}
	a.service = service
	// Восстановление сессии
	if user := service.GetCurrentUser(); user != nil {
		a.currentUser = &api.LoginResult{User: user}
	}
}

// LoginUser – вход в систему
func (a *App) LoginUser(login, password string) (*api.LoginResult, error) {
	res, err := a.service.Login(login, password)
	if err != nil {
		return nil, err
	}
	a.currentUser = res
	return res, nil
}

// LogoutUser – выход
func (a *App) LogoutUser() {
	a.service.Logout()
	a.currentUser = nil
}

// RegisterUser – регистрация нового пользователя с организацией
func (a *App) RegisterUser(login, password, name, organization string) error {
	return a.service.Register(login, password, name, organization)
}

// GetCurrentUser – возвращает текущего авторизованного пользователя
func (a *App) GetCurrentUser() *api.LoginResult {
	if a.currentUser == nil {
		return nil
	}
	return a.currentUser
}

// SelectFile – открывает диалог выбора файла
func (a *App) SelectFile() (string, error) {
	filePath, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Выберите аудио или текстовый файл",
		Filters: []runtime.FileFilter{
    		{DisplayName: "Поддерживаемые файлы (*.wav, *.txt, *.mp4, *.mov, *.mkv, *.avi)", 
     		Pattern: "*.wav;*.txt;*.mp4;*.mov;*.mkv;*.avi"},
		},
	})
	if err != nil {
		return "", err
	}
	return filePath, nil
}

// AnalyzeMediaByPath – анализ файла любого поддерживаемого формата (текст, аудио, видео)
func (a *App) AnalyzeMediaByPath(filePath string) (*api.AnalyzeFileResult, error) {
	return a.service.AnalyzeMediaByPath(filePath)
}

// AnalyzeFileByPath – анализ текстового файла
func (a *App) AnalyzeFileByPath(filePath string) (*api.AnalyzeFileResult, error) {
	return a.service.AnalyzeFile(filePath)
}

// AnalyzeAudioByPath – анализ аудиофайла (WAV)
func (a *App) AnalyzeAudioByPath(filePath string) (*api.AnalyzeFileResult, error) {
	return a.service.AnalyzeAudio(filePath)
}

// GetCalendarEvents – возвращает события календаря
func (a *App) GetCalendarEvents() ([]api.CalendarEvent, error) {
	return a.service.GetCalendarEvents()
}

// AddCalendarEvent – добавляет событие в календарь вручную
func (a *App) AddCalendarEvent(title, startStr, endStr, assignee string) error {
	start, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		return fmt.Errorf("неверный формат даты начала: %w", err)
	}
	var end time.Time
	if endStr != "" {
		end, err = time.Parse(time.RFC3339, endStr)
		if err != nil {
			return fmt.Errorf("неверный формат даты окончания: %w", err)
		}
	} else {
		end = start.Add(1 * time.Hour)
	}
	return a.service.AddCalendarEvent(title, start, end, assignee)
}

// ClearAllEvents – удаляет все события из календаря
func (a *App) ClearAllEvents() error {
	return a.service.ClearAllEvents()
}

// MarkEventCompleted – отмечает событие выполненным или снимает отметку
func (a *App) MarkEventCompleted(eventID int, completed bool) error {
	return a.service.MarkEventCompleted(eventID, completed)
}

// ExportCalendar – возвращает ICS-строку всех событий календаря
func (a *App) ExportCalendar() (string, error) {
	return a.service.ExportCalendar()
}

// SaveFileDialog – показывает диалог сохранения файла и возвращает выбранный путь
func (a *App) SaveFileDialog(title, defaultName string) (string, error) {
	return runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           title,
		DefaultFilename: defaultName,
		Filters: []runtime.FileFilter{
			{DisplayName: "iCalendar (*.ics)", Pattern: "*.ics"},
		},
	})
}

// WriteFile – записывает содержимое в файл по указанному пути
func (a *App) WriteFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

// SearchTranscripts – полнотекстовый поиск по архиву
func (a *App) SearchTranscripts(query string) ([]api.SearchResult, error) {
	return a.service.SearchTranscripts(query)
}

// SearchTranscriptsAdvanced – расширенный поиск с фильтрами
func (a *App) SearchTranscriptsAdvanced(query, dateFrom, dateTo, assignee, topic string) ([]api.SearchResult, error) {
	return a.service.SearchTranscriptsAdvanced(query, dateFrom, dateTo, assignee, topic)
}

// GetProtocolsList – список протоколов
func (a *App) GetProtocolsList() ([]string, error) {
	return a.service.GetProtocolsList()
}

// ReadProtocol – чтение содержимого протокола
func (a *App) ReadProtocol(filename string) (string, error) {
	return a.service.ReadProtocol(filename)
}

// SaveProtocol – сохранение отредактированного протокола
func (a *App) SaveProtocol(filename, content string) error {
	return a.service.SaveProtocol(filename, content)
}

// GetNotifications – список персональных уведомлений
func (a *App) GetNotifications() ([]string, error) {
	return a.service.GetNotifications()
}

// ReadNotification – чтение содержимого уведомления
func (a *App) ReadNotification(filename string) (string, error) {
	return a.service.ReadNotification(filename)
}

// GetTasks – возвращает список задач для канбан-доски
func (a *App) GetTasks() ([]api.Task, error) {
	return a.service.GetTasks()
}

// AddTask – добавляет новую задачу вручную
func (a *App) AddTask(title, priority, due, assignee string) error {
	return a.service.AddTask(title, priority, due, assignee)
}

// UpdateTaskStatus – обновляет статус задачи
func (a *App) UpdateTaskStatus(taskID int, newStatus string) error {
	return a.service.UpdateTaskStatus(taskID, newStatus)
}

// GetStatistics – возвращает агрегированную статистику
func (a *App) GetStatistics() (*api.Statistics, error) {
	return a.service.GetStatistics()
}