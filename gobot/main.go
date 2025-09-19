package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	_ "github.com/mattn/go-sqlite3"
)

type Task struct {
	ID          int
	UserID      int64
	Description string
	Deadline    time.Time
	Reminded    bool
}

func main() {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN не установлен")
	}

	db, err := sql.Open("sqlite3", "tasks.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	initDB(db)

	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Panic(err)
	}
	bot.Debug = false
	log.Printf("Авторизация: %s", bot.Self.UserName)

	go reminderLoop(bot, db)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil {
			continue
		}
		userID := update.Message.From.ID
		text := update.Message.Text
		if text == "/start" {
			msg := "Привет! Я бот-задачник. Отправь мне задачу в формате:\n<текст задачи> ; <дедлайн ГГГГ-ММ-ДД ЧЧ:ММ>\nПример: Купить хлеб ; 2025-09-20 18:00\n\nКоманды:\n/tasks — список задач\n/done <id> — удалить задачу"
			bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, msg))
		} else if text == "/tasks" {
			tasks := getTasks(db, userID)
			if len(tasks) == 0 {
				bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "У вас нет задач."))
				continue
			}
			var sb strings.Builder
			sb.WriteString("Ваши задачи:\n")
			for _, t := range tasks {
				sb.WriteString(fmt.Sprintf("%d. %s — до %s\n", t.ID, t.Description, t.Deadline.Format("2006-01-02 15:04")))
			}
			bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, sb.String()))
		} else if strings.HasPrefix(text, "/done") {
			parts := strings.Fields(text)
			if len(parts) != 2 {
				bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Используйте: /done <id задачи>"))
				continue
			}
			id := parts[1]
			res, err := db.Exec("DELETE FROM tasks WHERE id=? AND user_id=?", id, userID)
			if err != nil {
				bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Ошибка удаления задачи."))
				continue
			}
			n, _ := res.RowsAffected()
			if n > 0 {
				bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Задача удалена."))
			} else {
				bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Задача не найдена."))
			}
		} else if strings.Contains(text, ";") {
			parts := strings.SplitN(text, ";", 2)
			desc := strings.TrimSpace(parts[0])
			deadline := strings.TrimSpace(parts[1])
			dt, err := time.Parse("2006-01-02 15:04", deadline)
			if err != nil {
				bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Неверный формат даты. Пример: 2025-09-20 18:00"))
				continue
			}
			_, err = db.Exec("INSERT INTO tasks (user_id, description, deadline, reminded) VALUES (?, ?, ?, 0)", userID, desc, dt.Format("2006-01-02 15:04"))
			if err != nil {
				bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Ошибка добавления задачи."))
				continue
			}
			bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Задача добавлена!"))
		} else {
			bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Неизвестная команда. Используйте /start для справки."))
		}
	}
}

func initDB(db *sql.DB) {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS tasks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER,
		description TEXT,
		deadline TEXT,
		reminded INTEGER DEFAULT 0
	)`)
	if err != nil {
		log.Fatal(err)
	}
}

func getTasks(db *sql.DB, userID int64) []Task {
	rows, err := db.Query("SELECT id, description, deadline FROM tasks WHERE user_id=? ORDER BY deadline", userID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var tasks []Task
	for rows.Next() {
		var t Task
		var deadline string
		if err := rows.Scan(&t.ID, &t.Description, &deadline); err == nil {
			t.Deadline, _ = time.Parse("2006-01-02 15:04", deadline)
			tasks = append(tasks, t)
		}
	}
	return tasks
}

func reminderLoop(bot *tgbotapi.BotAPI, db *sql.DB) {
	for {
		now := time.Now()
		rows, err := db.Query("SELECT id, user_id, description, deadline FROM tasks WHERE reminded=0")
		if err != nil {
			time.Sleep(time.Minute)
			continue
		}
		for rows.Next() {
			var id int
			var userID int64
			var desc, deadline string
			if err := rows.Scan(&id, &userID, &desc, &deadline); err == nil {
				dt, _ := time.Parse("2006-01-02 15:04", deadline)
				if now.After(dt.Add(-10*time.Minute)) && now.Before(dt) {
					msg := fmt.Sprintf("Напоминание! Задача: %s\nДедлайн через 10 минут: %s", desc, deadline)
					bot.Send(tgbotapi.NewMessage(userID, msg))
					db.Exec("UPDATE tasks SET reminded=1 WHERE id=?", id)
				}
			}
		}
		rows.Close()
		time.Sleep(time.Minute)
	}
}
