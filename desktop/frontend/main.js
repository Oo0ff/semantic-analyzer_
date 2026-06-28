// Глобальные переменные
let currentUser = null;
let currentMonth = new Date().getMonth();
let currentYear = new Date().getFullYear();
let calendarEvents = [];
let allTasks = [];
let activeDayDate = null;
let notificationTimer = null;
let notifiedIds = new Set();

// ---------- Авторизация ----------
window.addEventListener('DOMContentLoaded', async () => {
    try {
        if (window.go && window.go.main && window.go.main.App.GetCurrentUser) {
            const user = await window.go.main.App.GetCurrentUser();
            if (user) {
                currentUser = user;
                showApp();
            }
        }
    } catch (e) {}
});

function showApp() {
    document.getElementById('login-screen').style.display = 'none';
    document.getElementById('app').style.display = 'block';
    const userDisplay = document.getElementById('userDisplay');
    if (userDisplay && currentUser) {
        let org = currentUser.Organization || '';
        let name = currentUser.Name || currentUser.Login || 'Пользователь';
        userDisplay.textContent = org ? `${name} (${org})` : name;
    }
    if (window.Notification && Notification.permission !== 'granted' && Notification.permission !== 'denied') {
        Notification.requestPermission();
    }
    startNotificationChecker();
}

function showRegister() {
    document.querySelector('.login-box').style.display = 'none';
    document.querySelector('.register-box').style.display = 'block';
}
function hideRegister() {
    document.querySelector('.register-box').style.display = 'none';
    document.querySelector('.login-box').style.display = 'block';
}

async function login() {
    const loginInput = document.getElementById('loginInput').value;
    const passwordInput = document.getElementById('passwordInput').value;
    const errorEl = document.getElementById('loginError');
    errorEl.textContent = '';
    try {
        const result = await window.go.main.App.LoginUser(loginInput, passwordInput);
        currentUser = result;
        showApp();
    } catch (e) {
        errorEl.textContent = 'Ошибка: ' + e;
    }
}

async function register() {
    const login = document.getElementById('regLogin').value;
    const name = document.getElementById('regName').value;
    const password = document.getElementById('regPassword').value;
    const organization = document.getElementById('regOrganization')?.value || '';
    const errorEl = document.getElementById('regError');
    errorEl.textContent = '';
    try {
        await window.go.main.App.RegisterUser(login, password, name, organization);
        alert('Регистрация успешна. Теперь войдите.');
        hideRegister();
    } catch (e) {
        errorEl.textContent = 'Ошибка: ' + e;
    }
}

async function logout() {
    try { await window.go.main.App.LogoutUser(); } catch (e) {}
    currentUser = null;
    clearInterval(notificationTimer);
    document.getElementById('app').style.display = 'none';
    document.getElementById('login-screen').style.display = 'block';
}

// ---------- Вкладки ----------
function openTab(evt, tabName) {
    const tabcontent = document.getElementsByClassName("tabcontent");
    for (let i = 0; i < tabcontent.length; i++) tabcontent[i].style.display = "none";
    const tablinks = document.getElementsByClassName("tablink");
    for (let i = 0; i < tablinks.length; i++) tablinks[i].classList.remove("active");
    document.getElementById(tabName).style.display = "block";
    evt.currentTarget.classList.add("active");
    if (tabName === 'calendar') loadCalendar();
    if (tabName === 'notifications') loadNotifications();
    if (tabName === 'protocols') loadProtocols();
    if (tabName === 'tasks') loadTasks();
    if (tabName === 'analytics') loadAnalytics();
    if (tabName === 'notes') loadNotes();
}

// ---------- Защита от XSS ----------
function escapeHtml(text) {
    const map = { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#039;' };
    return text.replace(/[&<>"']/g, m => map[m]);
}

// ---------- Анализ ----------
async function selectAndAnalyze() {
    try {
        const filePath = await window.go.main.App.SelectFile();
        if (!filePath) { alert("Файл не выбран"); return; }
        // Единый вызов для всех типов (текст, аудио, видео)
        const result = await window.go.main.App.AnalyzeMediaByPath(filePath);
        displayAnalysis(result);
        // Принудительно обновляем данные для других вкладок
        await loadCalendar();
        await loadTasks();
        await loadNotifications();
    } catch (e) { alert("Ошибка: " + e); }
}

function displayAnalysis(data) {
    let html = `<h3>Результат</h3><p><b>ID:</b> ${data.id}</p><p><b>Источник:</b> ${data.source}</p>`;
    const atomicMarkers = data.markers?.filter(m => m.is_atomic) || [];
    const compositeMarkers = data.markers?.filter(m => !m.is_atomic) || [];
    html += `<h4>Атомарные маркеры (${atomicMarkers.length})</h4>`;
    if (atomicMarkers.length > 0) {
        html += '<ul>';
        atomicMarkers.forEach(m => {
            const conf = (m.confidence / 1000000).toFixed(2);
            html += `<li>${m.type}: "${escapeHtml(m.text_span)}" (уверенность: ${conf})</li>`;
        });
        html += '</ul>';
    } else html += '<p>Не найдены</p>';
    html += `<h4>Композитные маркеры (${compositeMarkers.length})</h4>`;
    if (compositeMarkers.length > 0) {
        html += '<ul>';
        compositeMarkers.forEach(m => {
            const conf = (m.confidence / 1000000).toFixed(2);
            html += `<li>${m.type}: "${escapeHtml(m.text_span)}" (уверенность: ${conf})</li>`;
        });
        html += '</ul>';
    } else html += '<p>Не найдены</p>';
    html += `<h4>Темы</h4><ul>`;
    if (data.topics) {
        data.topics.forEach(t => html += `<li>${escapeHtml(t.label)} (${(t.confidence * 100).toFixed(0)}%)</li>`);
    }
    html += `</ul><h4>Сводка</h4><p>${escapeHtml(data.summary || "—")}</p>`;
    document.getElementById('analysisResult').innerHTML = html;
}

// ---------- Календарь ----------
async function loadCalendar() {
    try {
        const events = await window.go.main.App.GetCalendarEvents();
        calendarEvents = events || [];
        renderCalendar();
    } catch (e) { alert("Ошибка загрузки календаря: " + e); }
}

function getTimeZoneOffset() {
    const offset = -new Date().getTimezoneOffset();
    const sign = offset >= 0 ? '+' : '-';
    const hours = String(Math.floor(Math.abs(offset) / 60)).padStart(2, '0');
    const mins = String(Math.abs(offset) % 60).padStart(2, '0');
    return sign + hours + ':' + mins;
}

async function addCalendarEvent() {
    const title = document.getElementById('newEventTitle').value;
    const startStrRaw = document.getElementById('newEventStart').value;
    const endStrRaw = document.getElementById('newEventEnd').value;
    const assignee = document.getElementById('newEventAssignee').value;
    if (!title || !startStrRaw) { alert("Название и дата начала обязательны"); return; }
    const startStr = startStrRaw + ":00" + getTimeZoneOffset();
    let endStr = "";
    if (endStrRaw) endStr = endStrRaw + ":00" + getTimeZoneOffset();
    try {
        await window.go.main.App.AddCalendarEvent(title, startStr, endStr, assignee);
        document.getElementById('newEventTitle').value = '';
        document.getElementById('newEventStart').value = '';
        document.getElementById('newEventEnd').value = '';
        document.getElementById('newEventAssignee').value = '';
        await loadCalendar();
    } catch (e) { alert("Ошибка добавления события: " + e); }
}

async function clearAllEvents() {
    if (!confirm("Удалить ВСЕ события из календаря? Это действие необратимо.")) return;
    try {
        await window.go.main.App.ClearAllEvents();
        alert("Все события удалены.");
        await loadCalendar();
    } catch (e) { alert("Ошибка: " + e); }
}

async function exportCalendar() {
    try {
        const icsContent = await window.go.main.App.ExportCalendar();
        if (window.go && window.go.main && window.go.main.App.SaveFileDialog) {
            const filePath = await window.go.main.App.SaveFileDialog("Сохранить календарь", "calendar.ics");
            if (filePath) {
                await window.go.main.App.WriteFile(filePath, icsContent);
                alert("Календарь экспортирован в " + filePath);
            }
        } else {
            const blob = new Blob([icsContent], { type: 'text/calendar' });
            const a = document.createElement('a'); a.href = URL.createObjectURL(blob); a.download = 'calendar.ics'; a.click();
        }
    } catch (e) { alert("Ошибка экспорта: " + e); }
}

function renderCalendar() {
    const container = document.getElementById('calendarEvents');
    const monthNames = ["Январь", "Февраль", "Март", "Апрель", "Май", "Июнь", "Июль", "Август", "Сентябрь", "Октябрь", "Ноябрь", "Декабрь"];
    const dayNames = ["Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Вс"];
    let firstDay = new Date(currentYear, currentMonth, 1).getDay();
    firstDay = (firstDay === 0) ? 7 : firstDay;
    const daysInMonth = new Date(currentYear, currentMonth + 1, 0).getDate();
    const today = new Date();
    const todayStr = `${today.getFullYear()}-${String(today.getMonth()+1).padStart(2,'0')}-${String(today.getDate()).padStart(2,'0')}`;
    let html = `<div style="display:flex; gap:10px; margin-bottom:10px;">
        <button onclick="clearAllEvents()" style="background:#e74c3c; color:white; border:none; padding:6px 12px; border-radius:4px; cursor:pointer;">Удалить все события</button>
        <button onclick="exportCalendar()" style="background:#2980b9; color:white; border:none; padding:6px 12px; border-radius:4px; cursor:pointer;">Экспорт в ICS</button>
    </div><div class="calendar-header">
        <button onclick="changeMonth(-1)">←</button>
        <span>${monthNames[currentMonth]} ${currentYear}</span>
        <button onclick="changeMonth(1)">→</button>
    </div><table class="calendar-table"><tr>${dayNames.map(d => `<th>${d}</th>`).join('')}</tr>`;
    let day = 1;
    for (let row = 0; row < 6; row++) {
        html += '<tr>';
        for (let col = 0; col < 7; col++) {
            if (row === 0 && col < firstDay - 1) { html += '<td></td>'; continue; }
            if (day > daysInMonth) { html += '<td></td>'; continue; }
            const dateStr = `${currentYear}-${String(currentMonth+1).padStart(2,'0')}-${String(day).padStart(2,'0')}`;
            const isPast = dateStr < todayStr;
            const dayEvents = calendarEvents.filter(ev => ev.start && ev.start.startsWith(dateStr));
            const dayTasks = allTasks.filter(t => t.due_date && t.due_date.startsWith(dateStr));
            html += `<td class="calendar-day${isPast ? ' past-day' : ''}" onclick="showDayEvents('${dateStr}')"><div class="day-number">${day}</div>`;
            dayEvents.slice(0, 3).forEach(ev => {
                const pastEvent = ev.completed || (ev.end && new Date(ev.end) < today);
                html += `<div class="event-dot${pastEvent ? ' past-event-dot' : ''}" title="${escapeHtml(ev.title)}">•</div>`;
            });
            if (dayEvents.length + dayTasks.length > 3) {
                html += `<div class="event-more">+${dayEvents.length + dayTasks.length - 3}</div>`;
            }
            html += '</td>';
            day++;
        }
        html += '</tr>';
        if (day > daysInMonth) break;
    }
    html += '</table><div id="dayEventsDetail"></div>';
    container.innerHTML = html;
    if (activeDayDate) showDayEvents(activeDayDate);
}

function changeMonth(delta) {
    currentMonth += delta;
    if (currentMonth < 0) { currentMonth = 11; currentYear--; }
    if (currentMonth > 11) { currentMonth = 0; currentYear++; }
    renderCalendar();
}

function showDayEvents(dateStr) {
    activeDayDate = dateStr;
    const dayEvents = calendarEvents.filter(ev => ev.start && ev.start.startsWith(dateStr));
    const dayTasks = allTasks.filter(t => t.due_date && t.due_date.startsWith(dateStr));
    const today = new Date();
    let html = `<h4>События и задачи на ${dateStr}</h4>`;
    if (dayEvents.length === 0 && dayTasks.length === 0) {
        html += '<p>Нет событий или задач</p>';
    } else {
        if (dayEvents.length > 0) {
            html += '<h5>События</h5><ul>';
            dayEvents.forEach(ev => {
                const endTime = ev.end ? new Date(ev.end) : null;
                const isPast = ev.completed || (endTime && endTime < today);
                const timeStr = ev.start ? new Date(ev.start).toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' }) : '';
                html += `<li class="${isPast ? 'past-event' : ''}" style="display:flex; align-items:center; gap:8px;">
                    <input type="checkbox" onchange="toggleEventCompleted(${ev.id}, this.checked)" ${ev.completed ? 'checked' : ''}>
                    <span style="${ev.completed ? 'text-decoration: line-through; color: gray;' : ''}"><b>${timeStr}</b> ${escapeHtml(ev.title)}</span>
                </li>`;
            });
            html += '</ul>';
        }
        if (dayTasks.length > 0) {
            html += '<h5>Задачи</h5><ul>';
            dayTasks.forEach(task => {
                const statusText = task.status === 'done' ? '(Выполнено)' : task.status === 'in_progress' ? '(В работе)' : '(К выполнению)';
                let timeDisplay = '';
                if (task.due_date) {
                    const dt = new Date(task.due_date);
                    if (!isNaN(dt.getTime())) {
                        const timePart = dt.toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' });
                        if (timePart !== '00:00' && timePart !== '23:59') {
                            timeDisplay = ` ${timePart}`;
                        }
                    }
                }
                html += `<li style="margin-bottom:4px;">
                    <span>${escapeHtml(task.title)}</span>
                    ${timeDisplay ? `<small>${timeDisplay}</small>` : ''}
                    <small> ${statusText}</small>
                    ${task.assignee ? `<small> (${escapeHtml(task.assignee)})</small>` : ''}
                </li>`;
            });
            html += '</ul>';
        }
    }
    document.getElementById('dayEventsDetail').innerHTML = html;
}

async function toggleEventCompleted(eventId, completed) {
    try {
        await window.go.main.App.MarkEventCompleted(eventId, completed);
        const ev = calendarEvents.find(e => e.id === eventId);
        if (ev) ev.completed = completed;
        renderCalendar();
    } catch (e) {
        alert("Ошибка: " + e);
        renderCalendar();
    }
}

// ---------- Протоколы ----------
async function loadProtocols() {
    try {
        const list = await window.go.main.App.GetProtocolsList();
        list.sort((a, b) => b.localeCompare(a));
        const maxShow = 3;
        let html = '<ul id="protocolShortList">';
        list.slice(0, maxShow).forEach(f => html += `<li><a href="#" onclick="viewProtocol('${f}')">${escapeHtml(f)}</a></li>`);
        html += '</ul>';
        if (list.length > maxShow) {
            html += `<div id="protocolMore" style="display:none;"><ul>`;
            list.slice(maxShow).forEach(f => html += `<li><a href="#" onclick="viewProtocol('${f}')">${escapeHtml(f)}</a></li>`);
            html += `</ul></div>`;
            html += `<button id="showAllProtocolsBtn" onclick="toggleAllProtocols()">Показать все</button>`;
        }
        html += '<div id="protocolContent"></div>';
        document.getElementById('protocolsList').innerHTML = html;
    } catch (e) { alert("Ошибка: " + e); }
}

function toggleAllProtocols() {
    const more = document.getElementById('protocolMore');
    const btn = document.getElementById('showAllProtocolsBtn');
    if (more.style.display === 'none') {
        more.style.display = 'block';
        btn.textContent = 'Скрыть';
    } else {
        more.style.display = 'none';
        btn.textContent = 'Показать все';
    }
}

async function viewProtocol(filename) {
    try {
        const content = await window.go.main.App.ReadProtocol(filename);
        document.getElementById('protocolContent').innerHTML = `
            <div style="margin: 20px 0 10px 0;">
                <button onclick="saveProtocol('${filename}')">Сохранить изменения</button>
                <button onclick="closeProtocol()" style="background:#95a5a6; margin-left:8px;">Закрыть</button>
            </div>
            <textarea id="protocolEditor" style="width:100%; height:400px;">${escapeHtml(content)}</textarea>`;
    } catch (e) { alert("Ошибка: " + e); }
}

function closeProtocol() {
    document.getElementById('protocolContent').innerHTML = '';
}

async function saveProtocol(filename) {
    const newContent = document.getElementById('protocolEditor').value;
    try {
        await window.go.main.App.SaveProtocol(filename, newContent);
        alert("Протокол сохранён.");
    } catch (e) { alert("Ошибка сохранения: " + e); }
}

// ---------- Уведомления ----------
async function loadNotifications() {
    try {
        const list = await window.go.main.App.GetNotifications();
        let html = '<ul>';
        if (list && list.length) {
            list.forEach(f => html += `<li><a href="#" onclick="viewNotification('${f}')">${escapeHtml(f)}</a></li>`);
        } else html += '<li>Нет уведомлений</li>';
        html += '</ul><div id="notificationContent"></div>';
        document.getElementById('notificationsList').innerHTML = html;
    } catch (e) { alert("Ошибка: " + e); }
}

async function viewNotification(filename) {
    try {
        const content = await window.go.main.App.ReadNotification(filename);
        if (!content) {
            document.getElementById('notificationContent').innerHTML = '<p>Пустое уведомление</p>';
            return;
        }
        const html = parseNotificationMarkdown(content, filename);
        document.getElementById('notificationContent').innerHTML = html;
    } catch (e) { alert("Ошибка: " + e); }
}

function parseNotificationMarkdown(md, filename) {
    const lines = md.split('\n');
    let html = '';
    let currentSection = null;
    let sectionTasks = [];
    let sectionEvents = [];
    const donePrefix = `notif_done_${filename}_`;
    function flushSection() {
        if (currentSection === 'tasks' && sectionTasks.length > 0) {
            html += '<h4>Задачи</h4><ul>';
            sectionTasks.forEach((task, idx) => {
                const storageKey = donePrefix + currentSection + '_' + idx;
                const isDone = localStorage.getItem(storageKey) === 'true';
                const textStyle = isDone ? 'text-decoration: line-through; color: gray;' : '';
                html += `<li style="margin-bottom:4px;">
                    <input type="checkbox" onchange="toggleTaskDone(this, '${storageKey}')" ${isDone ? 'checked' : ''}>
                    <span style="${textStyle}">${escapeHtml(task)}</span></li>`;
            });
            html += '</ul>';
        } else if (currentSection === 'events' && sectionEvents.length > 0) {
            html += '<h4>События</h4><ul>';
            sectionEvents.forEach(ev => html += `<li>${escapeHtml(ev)}</li>`);
            html += '</ul>';
        }
        sectionTasks = []; sectionEvents = []; currentSection = null;
    }
    for (let i = 0; i < lines.length; i++) {
        const line = lines[i];
        if (line.startsWith('## Назначенные задачи')) { flushSection(); currentSection = 'tasks'; continue; }
        else if (line.startsWith('## Предстоящие события')) { flushSection(); currentSection = 'events'; continue; }
        if (line.startsWith('# ')) { html += `<h3>${escapeHtml(line.substring(2))}</h3>`; continue; }
        if (currentSection === 'tasks' && line.trim().startsWith('- ')) sectionTasks.push(line.trim().substring(2).trim());
        else if (currentSection === 'events' && line.trim().startsWith('- ')) sectionEvents.push(line.trim().substring(2).trim());
        else if (!currentSection && line.trim() !== '') html += `<p>${escapeHtml(line)}</p>`;
    }
    flushSection();
    return html;
}

function toggleTaskDone(checkbox, storageKey) {
    const span = checkbox.nextElementSibling;
    if (checkbox.checked) {
        span.style.textDecoration = 'line-through'; span.style.color = 'gray';
        localStorage.setItem(storageKey, 'true');
    } else {
        span.style.textDecoration = ''; span.style.color = '';
        localStorage.setItem(storageKey, 'false');
    }
}

// ---------- Заметки ----------
async function loadNotes() {
    const notesList = JSON.parse(localStorage.getItem('notes_list') || '[]');
    let html = '<div style="display:flex; gap:20px;"><div style="width:200px;"><h4>Список заметок</h4>';
    html += '<button onclick="newNote()" style="margin-bottom:8px;">+ Новая заметка</button><ul>';
    notesList.forEach((note, idx) => {
        html += `<li><a href="#" onclick="openNote(${idx})">${escapeHtml(note.title || 'Без названия')}</a></li>`;
    });
    html += '</ul></div>';
    html += '<div style="flex:1;"><input type="text" id="noteTitle" placeholder="Название заметки" style="width:100%; margin-bottom:5px;" />';
    html += '<textarea id="notesEditor" style="width:100%; height:60vh; font-family: monospace;"></textarea>';
    html += '<button onclick="saveCurrentNote()">Сохранить</button><span id="notesStatus" style="margin-left:10px;"></span></div></div>';
    document.getElementById('notes').innerHTML = html;
    const activeIdx = localStorage.getItem('active_note_index');
    if (activeIdx !== null && notesList[activeIdx]) {
        openNote(parseInt(activeIdx));
    }
}

function newNote() {
    const notes = JSON.parse(localStorage.getItem('notes_list') || '[]');
    notes.push({ title: 'Новая заметка', content: '' });
    localStorage.setItem('notes_list', JSON.stringify(notes));
    localStorage.setItem('active_note_index', notes.length - 1);
    loadNotes();
}

function openNote(index) {
    const notes = JSON.parse(localStorage.getItem('notes_list') || '[]');
    if (!notes[index]) return;
    document.getElementById('noteTitle').value = notes[index].title || '';
    document.getElementById('notesEditor').value = notes[index].content || '';
    localStorage.setItem('active_note_index', index);
}

function saveCurrentNote() {
    const index = parseInt(localStorage.getItem('active_note_index') || '0');
    const notes = JSON.parse(localStorage.getItem('notes_list') || '[]');
    if (!notes[index]) return;
    notes[index].title = document.getElementById('noteTitle').value;
    notes[index].content = document.getElementById('notesEditor').value;
    localStorage.setItem('notes_list', JSON.stringify(notes));
    document.getElementById('notesStatus').textContent = 'Сохранено';
    setTimeout(() => document.getElementById('notesStatus').textContent = '', 2000);
    loadNotes();
}

// ---------- Канбан-доска задач ----------
async function loadTasks() {
    try {
        allTasks = await window.go.main.App.GetTasks();
        renderTasks();
    } catch (e) { alert("Ошибка загрузки задач: " + e); }
}

function renderTasks() {
    const container = document.getElementById('tasksBoard');
    if (!container) return;
    const columns = { 'todo': 'К выполнению', 'in_progress': 'В работе', 'done': 'Готово' };
    let html = '<div style="display:flex; gap:20px;">';
    for (const [status, title] of Object.entries(columns)) {
        html += `<div class="kanban-column" data-status="${status}" style="flex:1; background:#f0f0f0; padding:10px; border-radius:8px;">
            <h4>${title}</h4><ul class="sortable" data-status="${status}" style="list-style:none; padding:0; min-height:50px;">`;
        const tasksInColumn = allTasks.filter(t => t.status === status);
        tasksInColumn.forEach(task => {
            let dueDisplay = '';
            if (task.due_date) {
                const dt = new Date(task.due_date);
                if (!isNaN(dt.getTime())) {
                    const datePart = dt.toLocaleDateString('ru-RU', { day: 'numeric', month: 'long', year: 'numeric' });
                    const timePart = dt.toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' });
                    dueDisplay = `Срок: ${datePart}`;
                    if (timePart !== '00:00' && timePart !== '23:59') {
                        dueDisplay += ` ${timePart}`;
                    }
                } else {
                    dueDisplay = `Срок: ${task.due_date.slice(0,10)}`;
                }
            }
            html += `<li class="task-card" data-id="${task.id}" style="background:white; margin-bottom:6px; padding:8px; border-radius:4px; cursor:grab; box-shadow:0 1px 3px rgba(0,0,0,0.1);">
                <strong>${escapeHtml(task.title)}</strong>
                ${dueDisplay ? `<br><small>${dueDisplay}</small>` : ''}
                ${task.assignee ? `<br><small>${escapeHtml(task.assignee)}</small>` : ''}
            </li>`;
        });
        html += '</ul></div>';
    }
    html += '</div>';
    container.innerHTML = html;
    const sortables = document.querySelectorAll('.sortable');
    sortables.forEach(sortable => {
        new Sortable(sortable, {
            group: 'tasks',
            animation: 150,
            onEnd: function (evt) {
                const taskId = evt.item.dataset.id;
                const newStatus = evt.to.dataset.status;
                updateTaskStatus(taskId, newStatus);
            }
        });
    });
}

async function updateTaskStatus(taskId, newStatus) {
    try {
        await window.go.main.App.UpdateTaskStatus(parseInt(taskId), newStatus);
        const task = allTasks.find(t => t.id == taskId);
        if (task) task.status = newStatus;
    } catch (e) { alert("Ошибка обновления статуса: " + e); }
}

async function addTask() {
    const title = document.getElementById('newTaskTitle').value;
    if (!title) { alert("Введите название задачи"); return; }
    const priority = document.getElementById('newTaskPriority').value;
    const dueDate = document.getElementById('newTaskDue').value;
    const dueTime = document.getElementById('newTaskTime')?.value || '';
    const assignee = document.getElementById('newTaskAssignee').value;
    let dueStr = '';
    if (dueDate) {
        if (dueTime) {
            dueStr = dueDate + 'T' + dueTime + ':00' + getTimeZoneOffset();
        } else {
            dueStr = dueDate + 'T23:59:00' + getTimeZoneOffset();
        }
    }
    try {
        await window.go.main.App.AddTask(title, priority, dueStr, assignee);
        document.getElementById('newTaskTitle').value = '';
        document.getElementById('newTaskDue').value = '';
        if (document.getElementById('newTaskTime')) document.getElementById('newTaskTime').value = '';
        document.getElementById('newTaskAssignee').value = '';
        if (dueStr) {
            try {
                const eventTitle = `Дедлайн: ${title}`;
                await window.go.main.App.AddCalendarEvent(eventTitle, dueStr, '', assignee);
            } catch (e) { /* игнорируем */ }
        }
        await loadTasks();
        if (document.getElementById('calendar').style.display === 'block') await loadCalendar();
    } catch (e) { alert("Ошибка добавления задачи: " + e); }
}

// ---------- Аналитика ----------
async function loadAnalytics() {
    try {
        const stats = await window.go.main.App.GetStatistics();
        const ctx = document.getElementById('analyticsChart')?.getContext('2d');
        if (ctx && stats) {
            if (window.analyticsChartInstance) window.analyticsChartInstance.destroy();
            window.analyticsChartInstance = new Chart(ctx, {
                type: 'bar',
                data: {
                    labels: ['К выполнению', 'В работе', 'Завершено'],
                    datasets: [{
                        label: 'Количество',
                        data: [stats.todo_tasks, stats.in_progress_tasks, stats.done_tasks],
                        backgroundColor: ['#e74c3c', '#f1c40f', '#2ecc71']
                    }]
                },
                options: { responsive: true, maintainAspectRatio: true }
            });
        }
        document.getElementById('statsSummary').innerHTML = `
            <p>Всего задач: ${stats.total_tasks}</p>
            <p>Событий в этом месяце: ${stats.events_this_month}</p>`;
    } catch (e) { alert("Ошибка загрузки аналитики: " + e); }
}

// ---------- Поиск ----------
async function runSearch() {
    const query = document.getElementById('searchQuery').value.trim();
    if (!query) { alert("Введите поисковый запрос"); return; }
    const dateFrom = document.getElementById('searchDateFrom')?.value || '';
    const dateTo = document.getElementById('searchDateTo')?.value || '';
    const assignee = document.getElementById('searchAssignee')?.value || '';
    const topic = document.getElementById('searchTopic')?.value || '';
    try {
        const docs = await window.go.main.App.SearchTranscriptsAdvanced(query, dateFrom, dateTo, assignee, topic);
        let html = `<h3>Результаты по запросу «${escapeHtml(query)}»</h3>`;
        if (!docs || docs.length === 0) html += '<p>Ничего не найдено.</p>';
        else {
            html += `<p>Найдено ${docs.length} результатов</p>`;
            docs.forEach(doc => {
                html += `<div class="search-item"><b>${escapeHtml(doc.source || 'без названия')}</b>
                    <p>${highlight(doc.text || '', query)}</p></div>`;
            });
        }
        document.getElementById('searchResults').innerHTML = html;
    } catch (e) { alert("Ошибка поиска: " + e); }
}

function highlight(text, query) {
    if (!text) return '';
    const re = new RegExp(query.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), 'gi');
    return text.replace(re, '<mark>$&</mark>');
}

// ---------- Настольные напоминания ----------
function startNotificationChecker() {
    if (notificationTimer) clearInterval(notificationTimer);
    notificationTimer = setInterval(checkUpcoming, 60000);
    checkUpcoming();
}

function checkUpcoming() {
    if (!window.Notification || Notification.permission !== 'granted') return;
    const now = new Date();
    const thirtyMinutes = 30 * 60 * 1000;

    calendarEvents.forEach(ev => {
        if (ev.completed) return;
        const start = new Date(ev.start);
        const diff = start.getTime() - now.getTime();
        if (diff > 0 && diff <= thirtyMinutes) {
            const id = 'event_' + ev.id;
            if (!notifiedIds.has(id)) {
                new Notification('Напоминание о событии', { body: ev.title });
                notifiedIds.add(id);
            }
        }
    });

    allTasks.forEach(task => {
        if (task.status === 'done' || !task.due_date) return;
        const due = new Date(task.due_date);
        const diff = due.getTime() - now.getTime();
        if (diff <= thirtyMinutes) {
            const id = 'task_' + task.id;
            if (!notifiedIds.has(id)) {
                new Notification('Дедлайн задачи', { body: task.title });
                notifiedIds.add(id);
            }
        }
    });
}

// ---------- Drag-and-drop файлов ----------
document.addEventListener('DOMContentLoaded', () => {
    const dropZone = document.body;
    dropZone.addEventListener('dragover', (e) => e.preventDefault());
    dropZone.addEventListener('drop', async (e) => {
        e.preventDefault();
        const files = e.dataTransfer.files;
        if (files.length === 0) return;
        for (const file of files) {
            const path = file.path;
            if (path) {
                try {
                    // Единый вызов для любого типа (текст, аудио, видео)
                    const result = await window.go.main.App.AnalyzeMediaByPath(path);
                    displayAnalysis(result);
                } catch (err) { alert("Ошибка при обработке файла " + file.name + ": " + err); }
            }
        }
    });
});