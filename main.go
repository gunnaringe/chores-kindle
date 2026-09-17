package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"time"
)

var choresAPIToken string
var choresAPIURL = "https://chores.apphub.casa"
var familyID string

func init() {
	choresAPIToken = os.Getenv("CHORES_API_TOKEN")
	if choresAPIToken == "" {
		log.Fatal("CHORES_API_TOKEN environment variable not set")
	}

	// Fetch family ID on startup
	data, err := callChoresAPI("chores.v1.ChoresService/ListFamilies", map[string]interface{}{})
	if err != nil {
		log.Printf("Failed to fetch families: %v", err)
		familyID = "unknown"
		return
	}

	var resp struct {
		Families []map[string]interface{} `json:"families"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		log.Printf("Failed to parse families: %v", err)
		familyID = "unknown"
		return
	}

	if len(resp.Families) > 0 {
		if id, ok := resp.Families[0]["id"].(string); ok {
			familyID = id
			log.Printf("Using family: %s", familyID)
		}
	}
}

func main() {
	port := flag.String("port", "8999", "Port to listen on")
	flag.Parse()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		w.Write([]byte(htmlPage))
	})

	http.HandleFunc("/api/tasks", handleGetTasks)
	http.HandleFunc("/api/task/complete", handleCompleteTask)
	http.HandleFunc("/api/task/uncomplete", handleUncompleteTask)

	addr := ":" + *port
	log.Printf("Starting server on http://0.0.0.0%s\n", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func getStr(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func callChoresAPI(method string, req interface{}) ([]byte, error) {
	body, _ := json.Marshal(req)
	httpReq, _ := http.NewRequest("POST", fmt.Sprintf("%s/%s", choresAPIURL, method), bytes.NewReader(body))
	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", choresAPIToken))
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func handleGetTasks(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	if familyID == "" {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "family_id not initialized"})
		return
	}

	// Get today's date
	today := time.Now().Format("2006-01-02")

	// Fetch occurrences for today
	occReq := map[string]interface{}{
		"family_id":  familyID,
		"start_date": today,
		"end_date":   today,
	}
	occData, err := callChoresAPI("chores.v1.ChoresService/ListTaskOccurrences", occReq)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	// Parse response
	var occResp struct {
		Occurrences []map[string]interface{} `json:"occurrences"`
	}
	json.Unmarshal(occData, &occResp)

	// Normalize camelCase to snake_case and create task groups
	type NormalizedTask struct {
		TaskID      string                 `json:"task_id"`
		ChildID     string                 `json:"child_id"`
		DueDate     string                 `json:"due_date"`
		Title       string                 `json:"title"`
		Description string                 `json:"description"`
		ChildName   string                 `json:"child_name"`
		CompletedAt interface{}            `json:"completed_at"`
		Extra       map[string]interface{} `json:"_extra"`
	}

	type TaskGroup struct {
		ChildName string           `json:"child_name"`
		ChildID   string           `json:"child_id"`
		Tasks     []NormalizedTask `json:"tasks"`
	}

	groups := make(map[string]*TaskGroup)
	for _, occ := range occResp.Occurrences {
		task := NormalizedTask{
			TaskID:      getStr(occ, "taskId"),
			ChildID:     getStr(occ, "childId"),
			DueDate:     getStr(occ, "dueDate"),
			Title:       getStr(occ, "title"),
			Description: getStr(occ, "description"),
			ChildName:   getStr(occ, "childName"),
			CompletedAt: occ["completedAt"],
			Extra:       occ,
		}

		if groups[task.ChildID] == nil {
			groups[task.ChildID] = &TaskGroup{
				ChildName: task.ChildName,
				ChildID:   task.ChildID,
				Tasks:     []NormalizedTask{},
			}
		}
		groups[task.ChildID].Tasks = append(groups[task.ChildID].Tasks, task)
	}

	result := make([]*TaskGroup, 0, len(groups))
	for _, g := range groups {
		result = append(result, g)
	}

	// Sort alphabetically by child name
	sort.Slice(result, func(i, j int) bool {
		return result[i].ChildName < result[j].ChildName
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func handleCompleteTask(w http.ResponseWriter, r *http.Request) {
	var req map[string]string
	json.NewDecoder(r.Body).Decode(&req)

	completeReq := map[string]string{
		"task_id":  req["task_id"],
		"child_id": req["child_id"],
		"due_date": req["due_date"],
	}
	callChoresAPI("chores.v1.ChoresService/CompleteTask", completeReq)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleUncompleteTask(w http.ResponseWriter, r *http.Request) {
	var req map[string]string
	json.NewDecoder(r.Body).Decode(&req)

	uncompleteReq := map[string]string{
		"task_id":  req["task_id"],
		"child_id": req["child_id"],
		"due_date": req["due_date"],
	}
	callChoresAPI("chores.v1.ChoresService/UncompleteTask", uncompleteReq)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

const htmlPage = `<!DOCTYPE html>
<html>
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>Chores Kiosk</title>
	<style>
		* {
			box-sizing: border-box;
		}
		body {
			margin: 0;
			padding: 2px 4px;
			font-family: Arial, sans-serif;
			background: white;
			color: black;
			font-size: 17.6px;
			text-align: left;
		}
		table {
			border-collapse: collapse;
			width: 100%;
			text-align: left;
		}
		td {
			text-align: left;
		}
		h1 {
			font-size: 1.4em;
			margin: 3px 0 6px 0;
			text-align: left;
			font-weight: bold;
		}
		.child-group {
			margin-bottom: 12px;
			padding: 0;
		}
		.child-name {
			font-size: 1.25em;
			font-weight: bold;
			margin-bottom: 4px;
			padding: 0 4px;
			text-align: left;
		}
		.task-columns td {
			width: 50%;
			vertical-align: top;
			padding: 0 3px;
		}
		.column-header {
			font-size: 0.9em;
			font-weight: bold;
			margin-bottom: 2px;
			padding: 0 2px;
			color: #666;
		}
		.task {
			padding: 5px 3px;
			margin-bottom: 2px;
			cursor: pointer;
			font-size: 1.05em;
			text-align: left;
			line-height: 1.2;
			user-select: none;
		}
		.task:active {
			background: #f0f0f0;
		}
		.task.completed {
			color: #999;
		}
		.task.completed .task-title {
			text-decoration: line-through;
		}
		.task-checkbox {
			font-weight: bold;
		}
		.task-title {
			font-weight: 500;
		}
		.task-desc {
			font-size: 0.8em;
			color: #777;
		}
		.task.completed .task-desc {
			color: #aaa;
		}
		.loading {
			text-align: center;
			font-size: 1.1em;
			padding: 20px;
		}
		.error {
			text-align: center;
			font-size: 0.95em;
			padding: 20px;
			color: #d00;
		}
	</style>
</head>
<body>
	<div id="content" class="loading">Loading...</div>

	<script>
		document.getElementById('content').innerHTML = '<div style="text-align:center;padding:40px;font-size:1.5em;">Initializing...</div>';

		// Request screen wake lock to keep display awake
		var wakeLock = null;

		function requestWakeLock() {
			try {
				if (navigator.wakeLock && navigator.wakeLock.request) {
					navigator.wakeLock.request('screen').then(function(lock) {
						wakeLock = lock;
						console.log('Wake lock acquired');
					}).catch(function(err) {
						console.log('Wake lock failed:', err);
					});
				}
			} catch (err) {
				console.log('Wake lock unavailable:', err);
			}
		}

		// Request wake lock on load and periodically
		requestWakeLock();
		setInterval(requestWakeLock, 60000);

		function pad(n) {
			return n < 10 ? '0' + n : n;
		}

		function updateTime() {
			var now = new Date();
			document.getElementById('lastRefresh').textContent = now.getHours() + ':' + pad(now.getMinutes()) + ':' + pad(now.getSeconds());
		}

		function renderTask(task) {
			var completed = task.completed_at ? ' completed' : '';
			var checkbox = task.completed_at ? '✓' : '☐';
			var html = '<div class="task' + completed + '" onclick="toggleTask(\'' + escapeHtml(task.task_id) + '\', \'' + escapeHtml(task.child_id) + '\', \'' + escapeHtml(task.due_date) + '\')">';
			html += '<span class="task-checkbox">' + checkbox + '</span> ';
			html += '<span class="task-title">' + escapeHtml(task.title) + '</span>';

			var cents = task._extra && task._extra.amount && task._extra.amount.cents;
			var kr = cents ? Math.round(parseInt(cents, 10) / 100) : 0;
			var descLine = kr + ' kr';
			if (task.description && task.description.length > 0) {
				descLine += ' - ' + task.description;
			}
			html += '<div class="task-desc">' + escapeHtml(descLine) + '</div>';

			html += '</div>';
			return html;
		}

		function loadTasks() {
			var xhr = new XMLHttpRequest();
			xhr.onload = function() {
				try {
					var data = JSON.parse(xhr.responseText);
					var html = '';

					for (var i = 0; i < data.length; i++) {
						var group = data[i];
						html += '<div class="child-group">';
						html += '<div class="child-name">' + escapeHtml(group.child_name) + '</div>';

						// Separate tasks by classification
						var mandatory = [];
						var optional = [];
						for (var j = 0; j < group.tasks.length; j++) {
							var task = group.tasks[j];
							var isMandatory = !task._extra || !task._extra.classification || task._extra.classification === 'TASK_CLASSIFICATION_MANDATORY';
							if (isMandatory) {
								mandatory.push(task);
							} else {
								optional.push(task);
							}
						}

						// Render in two columns using a table (old browsers don't do flexbox reliably)
						html += '<table class="task-columns"><tr>';

						html += '<td>';
						if (mandatory.length > 0) {
							html += '<div class="column-header">Må gjøre</div>';
							for (var j = 0; j < mandatory.length; j++) {
								html += renderTask(mandatory[j]);
							}
						}
						html += '</td>';

						html += '<td>';
						if (optional.length > 0) {
							html += '<div class="column-header">Kan gjøre</div>';
							for (var j = 0; j < optional.length; j++) {
								html += renderTask(optional[j]);
							}
						}
						html += '</td>';

						html += '</tr></table>';
						html += '</div>'; // end child-group
					}

					if (!data || data.length === 0) {
						html = '<div style="text-align: center; padding: 40px; font-size: 1.5em;">No tasks today!</div>';
					}

					document.getElementById('content').innerHTML = html;
				} catch (e) {
					document.getElementById('content').innerHTML = '<div class="error">Error parsing tasks</div>';
					console.error(e);
				}
			};
			xhr.onerror = function() {
				document.getElementById('content').innerHTML = '<div class="error">Error loading tasks</div>';
			};
			xhr.open('GET', '/api/tasks?_=' + new Date().getTime(), true);
			xhr.setRequestHeader('Cache-Control', 'no-cache');
			xhr.send();
		}

		function toggleTask(taskID, childID, dueDate) {
			// Find the task element to check if it's completed
			var taskEl = event.currentTarget || event.target;
			var isCompleted = taskEl.classList.contains('completed');
			var endpoint = isCompleted ? '/api/task/uncomplete' : '/api/task/complete';

			var xhr = new XMLHttpRequest();
			xhr.onload = function() {
				loadTasks();
			};
			xhr.onerror = function() {
				console.error('Failed to toggle task');
			};
			xhr.open('POST', endpoint, true);
			xhr.setRequestHeader('Content-Type', 'application/json');
			xhr.send(JSON.stringify({
				task_id: taskID,
				child_id: childID,
				due_date: dueDate
			}));
		}

		function escapeHtml(text) {
			var div = document.createElement('div');
			div.textContent = text;
			return div.innerHTML;
		}

		loadTasks();
		setInterval(loadTasks, 60000);
		setInterval(updateTime, 1000);
	</script>
</body>
</html>
`
