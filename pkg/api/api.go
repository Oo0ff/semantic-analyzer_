package api

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"semantic-analyzer/cmd/pipeline"
	"semantic-analyzer/internal/auth"
	"semantic-analyzer/internal/calendar"
	"semantic-analyzer/internal/domain"
	"semantic-analyzer/internal/notifications"
	"semantic-analyzer/internal/search"
	"semantic-analyzer/pkg/config"
)

// ---------- Модели данных, используемые API ----------

// Task представляет задачу в канбан-доске.
type Task struct {
	ID           int    `json:"id"`
	Title        string `json:"title"`
	Status       string `json:"status"`        // "todo", "in_progress", "done"
	Priority     string `json:"priority"`
	Assignee     string `json:"assignee"`
	DueDate      string `json:"due_date"`
	TranscriptID string `json:"transcript_id"`
}

// Statistics содержит агрегированную информацию для панели аналитики.
type Statistics struct {
	TotalTasks       int `json:"total_tasks"`
	DoneTasks        int `json:"done_tasks"`
	InProgressTasks  int `json:"in_progress_tasks"`
	TodoTasks        int `json:"todo_tasks"`
	EventsThisMonth  int `json:"events_this_month"`
}

// ---------- Сервис ----------

// Service объединяет всю бизнес-логику.
type Service struct {
	cfg         *config.Config
	pipeline    *pipeline.Pipeline
	authStore   *auth.Store
	currentUser *auth.User
	notifier    *notifications.Notifier
	mu          sync.Mutex
	tasks       []Task
	nextTaskID  int
}

func NewService(configPath string) (*Service, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("загрузка конфигурации: %w", err)
	}
	p := pipeline.NewPipeline(cfg)
	if err := p.Initialize(); err != nil {
		return nil, fmt.Errorf("инициализация пайплайна: %w", err)
	}
	authStore, err := auth.NewStore("./auth.db")
	if err != nil {
		return nil, fmt.Errorf("инициализация auth: %w", err)
	}
	return &Service{
		cfg:        cfg,
		pipeline:   p,
		authStore:  authStore,
		notifier:   notifications.NewNotifier(cfg.Notifications.Dir),
		tasks:      make([]Task, 0),
		nextTaskID: 1,
	}, nil
}

// ---------- Авторизация ----------

type LoginResult struct {
	User  *auth.User `json:"user"`
	Token string     `json:"token"`
}

func (s *Service) Login(login, password string) (*LoginResult, error) {
	user, err := s.authStore.Authenticate(login, password)
	if err != nil {
		return nil, err
	}
	s.currentUser = user
	return &LoginResult{User: user, Token: "dummy_token"}, nil
}

func (s *Service) Logout() {
	s.currentUser = nil
}

func (s *Service) Register(login, password, name, organization string) error {
	return s.authStore.CreateUser(login, password, name, organization)
}

func (s *Service) GetCurrentUser() *auth.User {
	return s.currentUser
}

// ---------- Анализ ----------

type AnalyzeFileResult struct {
	ID         string               `json:"id"`
	Source     string               `json:"source"`
	Markers    []domain.Marker      `json:"markers"`
	Topics     []domain.TopicResult `json:"topics"`
	Summary    string               `json:"summary"`
	Statistics domain.Statistics    `json:"statistics"`
}

func (s *Service) AnalyzeFile(filePath string) (*AnalyzeFileResult, error) {
	if s.currentUser == nil {
		return nil, fmt.Errorf("необходимо авторизоваться")
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения файла: %w", err)
	}
	transcript := &domain.Transcript{
		ID:        fmt.Sprintf("%x", time.Now().UnixNano()),
		RawText:   string(data),
		Source:    filepath.Base(filePath) + " (text)",
		CreatedAt: time.Now(),
	}
	result, err := s.pipeline.Process(context.Background(), transcript)
	if err != nil {
		return nil, err
	}

	// Извлекаем список персон, загруженных из CSV (если есть)
	var customPersons []string
	if personsRaw, ok := transcript.Metadata["custom_persons"]; ok {
		if p, ok := personsRaw.([]string); ok {
			customPersons = p
		}
	}

	// Автоматически создаём задачи из ACTION-маркеров с поиском ответственного
	s.mu.Lock()
	for _, m := range result.Markers {
		if m.Type == "ACTION" || m.Type == "TASK" || m.Type == "TODO" {
			assignee := findResponsibleForMarker(m, transcript.ProcessedText, transcript.Sentences, customPersons)
			task := Task{
				ID:           s.nextTaskID,
				Title:        m.TextSpan,
				Status:       "todo",
				Priority:     "medium",
				Assignee:     assignee,
				DueDate:      "",
				TranscriptID: result.TranscriptID,
			}
			s.tasks = append(s.tasks, task)
			s.nextTaskID++
		}
	}
	// Генерация уведомлений
	var notifTasks []notifications.NotificationTask
	for _, t := range s.tasks {
		if t.TranscriptID == transcript.ID {
			notifTasks = append(notifTasks, notifications.NotificationTask{
				Title:    t.Title,
				Assignee: t.Assignee,
			})
		}
	}
	if err := s.notifier.Generate(result, notifTasks); err != nil {
		log.Printf("WARNING: failed to generate notifications: %v", err)
	}
	s.mu.Unlock()

	return &AnalyzeFileResult{
		ID:         result.TranscriptID,
		Source:     result.Transcript.Source,
		Markers:    result.Markers,
		Topics:     result.Topics,
		Summary:    result.Summary,
		Statistics: result.Statistics,
	}, nil
}

func (s *Service) AnalyzeAudio(filePath string) (*AnalyzeFileResult, error) {
	if s.currentUser == nil {
		return nil, fmt.Errorf("необходимо авторизоваться")
	}
	text, err := transcribeAudio(s.cfg.Audio.PythonPath, s.cfg.Audio.TranscribeScript, filePath, s.cfg.Audio.ModelPath)
	if err != nil {
		return nil, fmt.Errorf("ошибка транскрибирования: %w", err)
	}
	if strings.TrimSpace(text) == "" {
		text = "[речь не распознана]"
	}
	transcript := &domain.Transcript{
		ID:        fmt.Sprintf("%x", time.Now().UnixNano()),
		RawText:   text,
		Source:    filepath.Base(filePath) + " (audio)",
		CreatedAt: time.Now(),
	}
	result, err := s.pipeline.Process(context.Background(), transcript)
	if err != nil {
		return nil, err
	}

	var customPersons []string
	if personsRaw, ok := transcript.Metadata["custom_persons"]; ok {
		if p, ok := personsRaw.([]string); ok {
			customPersons = p
		}
	}

	s.mu.Lock()
	for _, m := range result.Markers {
		if m.Type == "ACTION" || m.Type == "TASK" || m.Type == "TODO" {
			assignee := findResponsibleForMarker(m, transcript.ProcessedText, transcript.Sentences, customPersons)
			task := Task{
				ID:           s.nextTaskID,
				Title:        m.TextSpan,
				Status:       "todo",
				Priority:     "medium",
				Assignee:     assignee,
				DueDate:      "",
				TranscriptID: result.TranscriptID,
			}
			s.tasks = append(s.tasks, task)
			s.nextTaskID++
		}
	}
	// Генерация уведомлений
	var notifTasks []notifications.NotificationTask
	for _, t := range s.tasks {
		if t.TranscriptID == transcript.ID {
			notifTasks = append(notifTasks, notifications.NotificationTask{
				Title:    t.Title,
				Assignee: t.Assignee,
			})
		}
	}
	if err := s.notifier.Generate(result, notifTasks); err != nil {
		log.Printf("WARNING: failed to generate notifications: %v", err)
	}
	s.mu.Unlock()

	return &AnalyzeFileResult{
		ID:         result.TranscriptID,
		Source:     result.Transcript.Source,
		Markers:    result.Markers,
		Topics:     result.Topics,
		Summary:    result.Summary,
		Statistics: result.Statistics,
	}, nil
}

func transcribeAudio(pythonPath, scriptPath, filePath, modelPath string) (string, error) {
	if pythonPath == "" {
		pythonPath = "python"
	}
	if scriptPath == "" {
		scriptPath = "../transcribe.py"
	}
	absScript, err := filepath.Abs(scriptPath)
	if err != nil {
		return "", fmt.Errorf("не удалось определить абсолютный путь к скрипту: %w", err)
	}
	absModel, err := filepath.Abs(modelPath)
	if err != nil {
    	return "", fmt.Errorf("не удалось определить абсолютный путь к модели: %w", err)
	}
	log.Printf("DEBUG: используем python=%q, скрипт=%q, аудио=%q, модель=%q", pythonPath, absScript, filePath, modelPath)

	if _, err := os.Stat(absScript); os.IsNotExist(err) {
		return "", fmt.Errorf("скрипт транскрипции не найден: %s", absScript)
	}

	cmd := exec.Command(pythonPath, absScript, filePath, absModel)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ошибка выполнения Python: %w\nВывод: %s", err, string(out))
	}
	text := strings.TrimSpace(string(out))
	if len(text) > 0 {
		preview := text
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		log.Printf("DEBUG: raw transcribed text (first 200 chars): %q", preview)
	} else {
		log.Println("DEBUG: transcribed text is empty")
	}
	if text == "" || strings.Contains(text, "no speech") {
		return "", nil
	}
	return text, nil
}

// findResponsibleForMarker ищет ответственного среди переданного списка персон
// в предложении, содержащем маркер, и в соседних (±2).
// Если в задаче есть местоимения "я"/"мне", ответственным считается ближайшее имя
// в предыдущем контексте.
func findResponsibleForMarker(m domain.Marker, fullText string, sentences []string, persons []string) string {
	if len(persons) == 0 {
		return ""
	}
	var bounds [][2]int
	offset := 0
	for _, s := range sentences {
		idx := strings.Index(fullText[offset:], s)
		if idx != -1 {
			start := offset + idx
			end := start + len(s)
			bounds = append(bounds, [2]int{start, end})
			offset = end
		}
	}
	sentIdx := -1
	for i, b := range bounds {
		if m.Start >= b[0] && m.End <= b[1] {
			sentIdx = i
			break
		}
	}
	if sentIdx == -1 {
		log.Printf("DEBUG findResponsibleForMarker: marker outside any sentence")
		return ""
	}

	// Проверяем наличие местоимений в текущем предложении
	markerText := fullText[bounds[sentIdx][0]:bounds[sentIdx][1]]
	hasPronoun := strings.Contains(markerText, "я ") || strings.Contains(markerText, "мне ") ||
		strings.Contains(markerText, "моя ") || strings.Contains(markerText, "моё ")

	// Функция поиска имени в заданном диапазоне предложений
	searchInRange := func(startIdx, endIdx int) string {
		for idx := startIdx; idx <= endIdx; idx++ {
			if idx >= 0 && idx < len(bounds) {
				neighbor := strings.ToLower(fullText[bounds[idx][0]:bounds[idx][1]])
				for _, name := range persons {
					if strings.Contains(neighbor, strings.ToLower(name)) {
						return name
					}
				}
			}
		}
		return ""
	}

	// Сначала ищем в текущем предложении и в ±2
	assignee := searchInRange(sentIdx-2, sentIdx+2)
	if assignee != "" {
		return assignee
	}

	// Если есть местоимение, ищем только в предыдущих предложениях (до -3)
	if hasPronoun {
		assignee = searchInRange(sentIdx-5, sentIdx-1) // более широкий контекст
		if assignee != "" {
			log.Printf("DEBUG findResponsibleForMarker: pronoun detected, assignee=%q", assignee)
			return assignee
		}
	}

	return ""
}
// convertVideoToWav вызывает FFmpeg для извлечения аудиодорожки в WAV 16 кГц моно.
// Возвращает путь к созданному временному файлу.
func convertVideoToWav(videoPath, ffmpegPath string) (string, error) {
	// Создаём временный файл с расширением .wav
	tmpFile, err := os.CreateTemp("", "sa_audio_*.wav")
	if err != nil {
		return "", fmt.Errorf("не удалось создать временный файл: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()

	// Аргументы FFmpeg:
	// -i input -vn (отключить видео) -acodec pcm_s16le (16-бит PCM) -ar 16000 (частота) -ac 1 (моно) -y (перезапись)
	cmd := exec.Command(ffmpegPath, "-i", videoPath, "-vn", "-acodec", "pcm_s16le", "-ar", "16000", "-ac", "1", "-y", tmpPath)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("ошибка конвертации FFmpeg: %w\n%s", err, stderr.String())
	}
	return tmpPath, nil
}
// AnalyzeMediaByPath автоматически определяет тип файла (текст, аудио, видео) и запускает анализ.
func (s *Service) AnalyzeMediaByPath(filePath string) (*AnalyzeFileResult, error) {
	if s.currentUser == nil {
		return nil, fmt.Errorf("необходимо авторизоваться")
	}
	ext := strings.ToLower(filepath.Ext(filePath))

	// Текстовый файл
	if ext == ".txt" {
		return s.AnalyzeFile(filePath)
	}

	// Аудиофайл (WAV) – напрямую
	if ext == ".wav" {
		return s.AnalyzeAudio(filePath)
	}

	// Видеофайлы – конвертируем в WAV, анализируем, удаляем временный
	videoExts := map[string]bool{".mp4": true, ".mov": true, ".mkv": true, ".avi": true, ".webm": true, ".flv": true}
	if videoExts[ext] {
		ffmpeg := s.cfg.Audio.FFmpegPath
		if ffmpeg == "" {
			ffmpeg = "ffmpeg" // значение по умолчанию
		}
		tmpWav, err := convertVideoToWav(filePath, ffmpeg)
		if err != nil {
			return nil, err
		}
		defer os.Remove(tmpWav) // очистка в любом случае

		return s.AnalyzeAudio(tmpWav)
	}

	return nil, fmt.Errorf("неподдерживаемый формат файла: %s", ext)
}

// ---------- Календарь ----------

type CalendarEvent struct {
	ID        int       `json:"id"`
	Title     string    `json:"title"`
	Start     time.Time `json:"start"`
	End       time.Time `json:"end"`
	Assignee  string    `json:"assignee"`
	Completed bool      `json:"completed"`
}

func (s *Service) GetCalendarEvents() ([]CalendarEvent, error) {
	if s.currentUser == nil {
		return nil, fmt.Errorf("необходимо авторизоваться")
	}
	repo, err := calendar.NewRepository(s.cfg.Calendar.DBPath)
	if err != nil {
		return nil, err
	}
	defer repo.Close()
	events, err := repo.List(time.Time{}, time.Time{})
	if err != nil {
		return nil, err
	}
	log.Printf("DEBUG GetCalendarEvents: user.Name=%q, user.Login=%q, total events=%d", 
		s.currentUser.Name, s.currentUser.Login, len(events))
	var uiEvents []CalendarEvent
	for _, ev := range events {
		// Если у события указан ответственный, показываем только если он соответствует текущему пользователю
		if ev.Assignee != "" {
			if !strings.EqualFold(ev.Assignee, s.currentUser.Name) && 
			   !strings.EqualFold(ev.Assignee, s.currentUser.Login) {
				continue
			}
		}
		uiEvents = append(uiEvents, CalendarEvent{
			ID:        ev.ID,
			Title:     ev.Title,
			Start:     ev.Start,
			End:       ev.End,
			Assignee:  ev.Assignee,
			Completed: ev.Completed,
		})
	}
	log.Printf("DEBUG GetCalendarEvents: filtered events=%d", len(uiEvents))
	return uiEvents, nil
}

func (s *Service) AddCalendarEvent(title string, start, end time.Time, assignee string) error {
	if s.currentUser == nil {
		return fmt.Errorf("необходимо авторизоваться")
	}
	repo, err := calendar.NewRepository(s.cfg.Calendar.DBPath)
	if err != nil {
		return err
	}
	defer repo.Close()
	ev := calendar.Event{
		Title:     title,
		Start:     start,
		End:       end,
		AllDay:    false,
		Assignee:  assignee,
		Completed: false,
	}
	_, err = repo.Insert(ev)
	return err
}

func (s *Service) ClearAllEvents() error {
	if s.currentUser == nil {
		return fmt.Errorf("необходимо авторизоваться")
	}
	repo, err := calendar.NewRepository(s.cfg.Calendar.DBPath)
	if err != nil {
		return err
	}
	defer repo.Close()
	return repo.DeleteAll()
}

func (s *Service) MarkEventCompleted(eventID int, completed bool) error {
	if s.currentUser == nil {
		return fmt.Errorf("необходимо авторизоваться")
	}
	repo, err := calendar.NewRepository(s.cfg.Calendar.DBPath)
	if err != nil {
		return err
	}
	defer repo.Close()
	return repo.MarkCompleted(eventID, completed)
}

func (s *Service) ExportCalendar() (string, error) {
	if s.currentUser == nil {
		return "", fmt.Errorf("необходимо авторизоваться")
	}
	repo, err := calendar.NewRepository(s.cfg.Calendar.DBPath)
	if err != nil {
		return "", err
	}
	defer repo.Close()
	events, err := repo.List(time.Time{}, time.Time{})
	if err != nil {
		return "", err
	}
	var ics strings.Builder
	ics.WriteString("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Semantic Analyzer//RU\r\n")
	for _, ev := range events {
		ics.WriteString("BEGIN:VEVENT\r\n")
		ics.WriteString(fmt.Sprintf("DTSTART:%s\r\n", ev.Start.Format("20060102T150405")))
		if !ev.End.IsZero() {
			ics.WriteString(fmt.Sprintf("DTEND:%s\r\n", ev.End.Format("20060102T150405")))
		}
		ics.WriteString(fmt.Sprintf("SUMMARY:%s\r\n", ev.Title))
		if ev.Assignee != "" {
			ics.WriteString(fmt.Sprintf("ORGANIZER;CN=%s:mailto:%s\r\n", ev.Assignee, ""))
		}
		ics.WriteString("END:VEVENT\r\n")
	}
	ics.WriteString("END:VCALENDAR\r\n")
	return ics.String(), nil
}

// ---------- Поиск ----------

type SearchResult struct {
	ID      string `json:"id"`
	Source  string `json:"source"`
	Text    string `json:"text"`
	Summary string `json:"summary"`
}

func (s *Service) SearchTranscripts(query string) ([]SearchResult, error) {
	if s.currentUser == nil {
		return nil, fmt.Errorf("необходимо авторизоваться")
	}
	return s.searchInternal(query, "", "", "", "")
}

func (s *Service) SearchTranscriptsAdvanced(query, dateFrom, dateTo, assignee, topic string) ([]SearchResult, error) {
	if s.currentUser == nil {
		return nil, fmt.Errorf("необходимо авторизоваться")
	}
	return s.searchInternal(query, dateFrom, dateTo, assignee, topic)
}

func (s *Service) searchInternal(query, dateFrom, dateTo, assignee, topic string) ([]SearchResult, error) {
	idx, err := search.NewIndexer(s.cfg.Search.IndexPath)
	if err != nil {
		return nil, err
	}
	defer idx.Close()
	results, err := idx.Search(query, 20)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	docs := search.ConvertResultToDocuments(results[0])
	var uiResults []SearchResult
	for _, doc := range docs {
		if assignee != "" && !strings.Contains(strings.ToLower(doc.Text), strings.ToLower(assignee)) {
			continue
		}
		uiResults = append(uiResults, SearchResult{
			ID:      doc.ID,
			Source:  doc.Source,
			Text:    doc.Text,
			Summary: doc.Summary,
		})
	}
	return uiResults, nil
}

// ---------- Протоколы ----------

func (s *Service) GetProtocolsList() ([]string, error) {
	if s.currentUser == nil {
		return nil, fmt.Errorf("необходимо авторизоваться")
	}
	files, err := filepath.Glob(s.cfg.Protocols.Dir + "/*.md")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, f := range files {
		names = append(names, filepath.Base(f))
	}
	return names, nil
}

func (s *Service) ReadProtocol(filename string) (string, error) {
	if s.currentUser == nil {
		return "", fmt.Errorf("необходимо авторизоваться")
	}
	data, err := os.ReadFile(filepath.Join(s.cfg.Protocols.Dir, filename))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (s *Service) SaveProtocol(filename, content string) error {
	if s.currentUser == nil {
		return fmt.Errorf("необходимо авторизоваться")
	}
	path := filepath.Join(s.cfg.Protocols.Dir, filename)
	return os.WriteFile(path, []byte(content), 0644)
}

// ---------- Уведомления ----------

func (s *Service) GetNotifications() ([]string, error) {
	if s.currentUser == nil {
		return nil, fmt.Errorf("необходимо авторизоваться")
	}
	files, err := filepath.Glob(s.cfg.Notifications.Dir + "/*.md")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, f := range files {
		base := filepath.Base(f)
		// Нормализуем имя файла: заменяем "_" на " " для сравнения
		normalized := strings.ReplaceAll(base, "_", " ")
		userName := strings.ToLower(s.currentUser.Name)
		userLogin := strings.ToLower(s.currentUser.Login)
		if strings.Contains(strings.ToLower(normalized), userName) ||
			strings.Contains(strings.ToLower(normalized), userLogin) {
			names = append(names, base)
		}
	}
	return names, nil
}

func (s *Service) ReadNotification(filename string) (string, error) {
	if s.currentUser == nil {
		return "", fmt.Errorf("необходимо авторизоваться")
	}
	data, err := os.ReadFile(filepath.Join(s.cfg.Notifications.Dir, filename))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ---------- Задачи (канбан) ----------

func (s *Service) GetTasks() ([]Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.currentUser == nil {
		return nil, fmt.Errorf("необходимо авторизоваться")
	}
	var filtered []Task
	for _, t := range s.tasks {
		// Показываем задачу, если assignee пустой или совпадает с именем/логином пользователя (без учёта регистра)
		if t.Assignee == "" ||
			strings.EqualFold(t.Assignee, s.currentUser.Name) ||
			strings.EqualFold(t.Assignee, s.currentUser.Login) {
			filtered = append(filtered, t)
		}
	}
	result := make([]Task, len(filtered))
	copy(result, filtered)
	return result, nil
}

func (s *Service) AddTask(title, priority, due, assignee string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	task := Task{
		ID:           s.nextTaskID,
		Title:        title,
		Status:       "todo",
		Priority:     priority,
		Assignee:     assignee,
		DueDate:      due,
		TranscriptID: "",
	}
	s.tasks = append(s.tasks, task)
	s.nextTaskID++
	return nil
}

func (s *Service) UpdateTaskStatus(taskID int, newStatus string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, t := range s.tasks {
		if t.ID == taskID {
			s.tasks[i].Status = newStatus
			return nil
		}
	}
	return fmt.Errorf("задача с id %d не найдена", taskID)
}

// ---------- Аналитика ----------

func (s *Service) GetStatistics() (*Statistics, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stats := &Statistics{}
	for _, t := range s.tasks {
		stats.TotalTasks++
		switch t.Status {
		case "done":
			stats.DoneTasks++
		case "in_progress":
			stats.InProgressTasks++
		default:
			stats.TodoTasks++
		}
	}

	repo, err := calendar.NewRepository(s.cfg.Calendar.DBPath)
	if err != nil {
		return nil, err
	}
	defer repo.Close()
	now := time.Now()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	endOfMonth := startOfMonth.AddDate(0, 1, -1)
	events, err := repo.List(startOfMonth, endOfMonth)
	if err != nil {
		return nil, err
	}
	stats.EventsThisMonth = len(events)
	return stats, nil
}