# Deployment Guide - VPS + Docker

Руководство по безопасному деплою бота на VPS с использованием Docker.

## Предварительные требования

На VPS должны быть установлены:
- Docker (20.10+)
- Docker Compose (1.29+)
- Git

## Пошаговая инструкция

### 1. Установка Docker на VPS (если еще не установлен)

```bash
# Обновить пакеты
sudo apt update

# Установить Docker
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh

# Добавить текущего пользователя в группу docker
sudo usermod -aG docker $USER

# Установить Docker Compose
sudo apt install docker-compose

# Перелогиниться для применения изменений группы
exit
```

После перелогина проверить установку:
```bash
docker --version
docker-compose --version
```

### 2. Клонирование репозитория на VPS

```bash
# Подключиться к VPS
ssh user@your-vps-ip

# Создать директорию для проекта
mkdir -p ~/apps
cd ~/apps

# Клонировать репозиторий
git clone https://github.com/Podogrev/corp-bullshifter-bot.git
cd corp-bullshifter-bot
```

### 3. Безопасная передача токенов

**Вариант 1: Создать .env файл напрямую на VPS (Рекомендуется)**

```bash
# На VPS создать .env файл
nano .env
```

Добавить содержимое:
```bash
TELEGRAM_BOT_TOKEN=your_telegram_token_here
CLAUDE_API_KEY=your_claude_api_key_here
CLAUDE_MODEL=claude-3-5-haiku-20241022
```

Сохранить (Ctrl+O, Enter, Ctrl+X).

**Вариант 2: Безопасная передача через scp**

```bash
# На локальной машине (не коммитить .env в git!)
# Временно создать файл .env.production с токенами
nano .env.production

# Передать файл на VPS
scp .env.production user@your-vps-ip:~/apps/corp-bullshifter-bot/.env

# УДАЛИТЬ локальный файл после передачи
rm .env.production
```

**Вариант 3: Использовать SSH и heredoc**

```bash
# На локальной машине
ssh user@your-vps-ip << 'EOF'
cd ~/apps/corp-bullshifter-bot
cat > .env << 'ENVFILE'
TELEGRAM_BOT_TOKEN=your_token_here
CLAUDE_API_KEY=your_key_here
CLAUDE_MODEL=claude-3-5-haiku-20241022
ENVFILE
EOF
```

### 4. Проверка безопасности .env файла

```bash
# На VPS проверить права доступа
ls -la .env

# Установить права только для владельца (рекомендуется)
chmod 600 .env

# Проверить что .env не отслеживается git
git status
# .env не должен появиться в списке (защищен .gitignore)
```

### 5. Сборка и запуск Docker контейнера

```bash
# Собрать образ
docker-compose build

# Запустить контейнер в фоновом режиме
docker-compose up -d

# Проверить что контейнер запустился
docker-compose ps

# Посмотреть логи
docker-compose logs -f
```

### 6. Управление ботом

**Посмотреть логи:**
```bash
docker-compose logs -f bot
```

**Остановить бота:**
```bash
docker-compose down
```

**Перезапустить бота:**
```bash
docker-compose restart
```

**Пересобрать и перезапустить после обновления кода:**
```bash
git pull
docker-compose down
docker-compose build
docker-compose up -d
```

**Посмотреть статус:**
```bash
docker-compose ps
```

### 7. Автозапуск при перезагрузке сервера

Docker Compose с `restart: unless-stopped` автоматически перезапустит контейнер при перезагрузке VPS.

Проверить:
```bash
sudo reboot
# После перезагрузки
docker-compose ps
```

### 8. Мониторинг

**Использование ресурсов:**
```bash
docker stats corp-bullshifter-bot
```

**Логи с временными метками:**
```bash
docker-compose logs -f --timestamps bot
```

## Безопасность

### Важные правила:

1. **.env файл никогда не коммитить в git**
   - Проверено через `.gitignore`
   - Всегда создавать только на сервере

2. **Ограничить доступ к .env файлу**
   ```bash
   chmod 600 .env
   ```

3. **Регулярно обновлять токены**
   - Периодически ротировать API ключи
   - При компрометации немедленно перевыпустить

4. **Firewall на VPS**
   ```bash
   # Разрешить только SSH и HTTPS
   sudo ufw allow 22
   sudo ufw allow 443
   sudo ufw enable
   ```

5. **Обновления**
   ```bash
   # Регулярно обновлять систему
   sudo apt update && sudo apt upgrade

   # Обновлять Docker образы
   docker-compose pull
   docker-compose up -d
   ```

## Troubleshooting

### Бот не запускается

```bash
# Проверить логи
docker-compose logs bot

# Проверить что .env файл существует и заполнен
cat .env

# Проверить что переменные загружены
docker-compose config
```

### Ошибки соединения с API

```bash
# Проверить сетевой доступ из контейнера
docker exec corp-bullshifter-bot ping -c 3 api.anthropic.com
docker exec corp-bullshifter-bot ping -c 3 api.telegram.org
```

### Очистка и переустановка

```bash
# Остановить и удалить контейнер
docker-compose down

# Удалить образ
docker rmi corp-bullshifter-bot_bot

# Пересобрать с нуля
docker-compose build --no-cache
docker-compose up -d
```

## Альтернатива: systemd service (без Docker)

Если не хочешь использовать Docker, можно настроить systemd service:

```bash
# Собрать бинарник
go build -o corp-bullshifter main.go

# Создать systemd unit файл
sudo nano /etc/systemd/system/corp-bullshifter.service
```

Содержимое:
```ini
[Unit]
Description=Corporate Bullshifter Telegram Bot
After=network.target

[Service]
Type=simple
User=your-username
WorkingDirectory=/home/your-username/apps/corp-bullshifter-bot
EnvironmentFile=/home/your-username/apps/corp-bullshifter-bot/.env
ExecStart=/home/your-username/apps/corp-bullshifter-bot/corp-bullshifter
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

Запуск:
```bash
sudo systemctl daemon-reload
sudo systemctl enable corp-bullshifter
sudo systemctl start corp-bullshifter
sudo systemctl status corp-bullshifter
```

## Полезные команды

```bash
# Проверить размер образа
docker images | grep corp-bullshifter

# Зайти внутрь контейнера
docker exec -it corp-bullshifter-bot sh

# Экспортировать логи в файл
docker-compose logs > bot-logs.txt

# Очистить неиспользуемые Docker ресурсы
docker system prune -a
```

## Обновление кода

```bash
cd ~/apps/corp-bullshifter-bot
git pull origin main
docker-compose down
docker-compose build
docker-compose up -d
docker-compose logs -f
```
