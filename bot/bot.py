from aiogram import Bot, Dispatcher, types, executor
import asyncio
import sqlite3
import logging
import os
from datetime import datetime, timedelta

API_TOKEN = os.getenv('TELEGRAM_BOT_TOKEN', 'YOUR_BOT_TOKEN_HERE')

logging.basicConfig(level=logging.INFO)

bot = Bot(token=API_TOKEN)
dp = Dispatcher(bot)

DB_PATH = 'tasks.db'

def init_db():
    conn = sqlite3.connect(DB_PATH)
    c = conn.cursor()
    c.execute('''CREATE TABLE IF NOT EXISTS tasks (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        user_id INTEGER,
        description TEXT,
        deadline TEXT,
        reminded INTEGER DEFAULT 0
    )''')
    conn.commit()
    conn.close()

@dp.message_handler(commands=['start'])
async def send_welcome(message: types.Message):
    await message.reply("Привет! Я бот-задачник. Отправь мне задачу в формате:\n<текст задачи> ; <дедлайн в формате ГГГГ-ММ-ДД ЧЧ:ММ>\nПример: Купить хлеб ; 2025-09-20 18:00\n\nКоманды:\n/tasks — список задач\n/done <id> — удалить задачу")

@dp.message_handler(commands=['tasks'])
async def list_tasks(message: types.Message):
    conn = sqlite3.connect(DB_PATH)
    c = conn.cursor()
    c.execute('SELECT id, description, deadline FROM tasks WHERE user_id=? ORDER BY deadline', (message.from_user.id,))
    rows = c.fetchall()
    conn.close()
    if not rows:
        await message.reply("У вас нет задач.")
        return
    text = "Ваши задачи:\n"
    for row in rows:
        text += f"{row[0]}. {row[1]} — до {row[2]}\n"
    await message.reply(text)

@dp.message_handler(commands=['done'])
async def done_task(message: types.Message):
    parts = message.text.split()
    if len(parts) != 2 or not parts[1].isdigit():
        await message.reply("Используйте: /done <id задачи>")
        return
    task_id = int(parts[1])
    conn = sqlite3.connect(DB_PATH)
    c = conn.cursor()
    c.execute('DELETE FROM tasks WHERE id=? AND user_id=?', (task_id, message.from_user.id))
    conn.commit()
    conn.close()
    await message.reply("Задача удалена (если была найдена).")

@dp.message_handler()
async def add_task(message: types.Message):
    if ';' not in message.text:
        await message.reply("Формат: <текст задачи> ; <дедлайн в формате ГГГГ-ММ-ДД ЧЧ:ММ>")
        return
    desc, deadline = map(str.strip, message.text.split(';', 1))
    try:
        dt = datetime.strptime(deadline, '%Y-%m-%d %H:%M')
    except ValueError:
        await message.reply("Неверный формат даты. Пример: 2025-09-20 18:00")
        return
    conn = sqlite3.connect(DB_PATH)
    c = conn.cursor()
    c.execute('INSERT INTO tasks (user_id, description, deadline) VALUES (?, ?, ?)', (message.from_user.id, desc, deadline))
    conn.commit()
    conn.close()
    await message.reply("Задача добавлена!")

async def reminder_loop():
    while True:
        now = datetime.now()
        conn = sqlite3.connect(DB_PATH)
        c = conn.cursor()
        c.execute('SELECT id, user_id, description, deadline FROM tasks WHERE reminded=0')
        rows = c.fetchall()
        for row in rows:
            task_id, user_id, desc, deadline = row
            dt = datetime.strptime(deadline, '%Y-%m-%d %H:%M')
            if now >= dt - timedelta(minutes=10) and now < dt:
                try:
                    await bot.send_message(user_id, f"Напоминание! Задача: {desc}\nДедлайн через 10 минут: {deadline}")
                except Exception as e:
                    logging.error(f"Ошибка отправки напоминания: {e}")
                c.execute('UPDATE tasks SET reminded=1 WHERE id=?', (task_id,))
        conn.commit()
        conn.close()
        await asyncio.sleep(60)

if __name__ == '__main__':
    init_db()
    loop = asyncio.get_event_loop()
    loop.create_task(reminder_loop())
    executor.start_polling(dp, skip_updates=True)
