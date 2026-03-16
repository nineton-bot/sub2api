# Sub2API 部署说明

## 部署信息

- **部署时间**: 2026-03-11
- **部署方式**: Docker Compose (Local Directory Version)
- **项目源**: https://github.com/nineton-bot/sub2api

## 端口配置

为避免与现有 CLIProxyAPI 服务冲突，使用以下端口配置：

| 服务 | 端口 | 说明 |
|------|------|------|
| Sub2API Web | 8090 | 主服务端口（原默认 8080） |
| PostgreSQL | 5432 | 仅容器内部访问 |
| Redis | 6379 | 仅容器内部访问 |

**CLIProxyAPI 占用端口**: 1455, 8085, 8317, 11451, 51121, 54545

## 安全凭证

已自动生成以下安全凭证：

```bash
# PostgreSQL 密码
POSTGRES_PASSWORD=24e52dbc1b880f3d2f34359fb7a3db2d

# JWT Secret (用于用户会话)
JWT_SECRET=7819a2e4ac03bc7c714f985002f9a82a5df7d3db8cac4b5c77683e5e7d8eebb6

# TOTP 加密密钥 (用于双因素认证)
TOTP_ENCRYPTION_KEY=338ca9f0cd9876d0017e59054be783b5132576cb2e4cf0fca49cd69180c0a76b
```

## 管理员账号

- **邮箱**: admin@sub2api.local
- **密码**: `8bfdadc5ca709b5e9962908525b45690`

> 首次登录后请立即修改密码

## 启动服务

```bash
cd /Users/lsfhost/.openclaw/agents/proxymaster/workspace/sub2api/deploy

# 启动所有服务
docker-compose -f docker-compose.local.yml up -d

# 查看日志
docker-compose -f docker-compose.local.yml logs -f sub2api

# 查看管理员密码
docker-compose -f docker-compose.local.yml logs sub2api | grep "admin password"
```

## 访问地址

- **Web 界面**: http://localhost:8090
- **API 端点**: http://localhost:8090/v1/

## 常用命令

```bash
# 查看服务状态
docker-compose -f docker-compose.local.yml ps

# 停止服务
docker-compose -f docker-compose.local.yml down

# 重启服务
docker-compose -f docker-compose.local.yml restart

# 查看所有日志
docker-compose -f docker-compose.local.yml logs -f

# 更新到最新版本
docker-compose -f docker-compose.local.yml pull
docker-compose -f docker-compose.local.yml up -d
```

## 数据备份

数据存储在本地目录，便于备份和迁移：

```bash
# 备份整个部署
cd /Users/lsfhost/.openclaw/agents/proxymaster/workspace/sub2api
tar czf sub2api-backup-$(date +%Y%m%d).tar.gz deploy/

# 仅备份数据
cd deploy
tar czf data-backup-$(date +%Y%m%d).tar.gz data/ postgres_data/ redis_data/
```

## 迁移到新服务器

```bash
# 1. 停止服务
docker-compose -f docker-compose.local.yml down

# 2. 打包整个 deploy 目录
cd ..
tar czf sub2api-complete.tar.gz deploy/

# 3. 传输到新服务器
scp sub2api-complete.tar.gz user@new-server:/path/

# 4. 在新服务器上解压并启动
tar xzf sub2api-complete.tar.gz
cd deploy/
docker-compose -f docker-compose.local.yml up -d
```

## 配置说明

### 运行模式

当前使用 `RUN_MODE=standard` (完整 SaaS 模式)

如需简化模式（隐藏计费功能），修改 `.env`:
```bash
RUN_MODE=simple
SIMPLE_MODE_CONFIRM=true  # 生产环境必需
```

### 安全配置

当前配置允许 HTTP 和私有 IP（适合开发/测试）：
```bash
SECURITY_URL_ALLOWLIST_ENABLED=false
SECURITY_URL_ALLOWLIST_ALLOW_INSECURE_HTTP=true
SECURITY_URL_ALLOWLIST_ALLOW_PRIVATE_HOSTS=true
```

**生产环境建议**:
- 启用 URL 白名单: `SECURITY_URL_ALLOWLIST_ENABLED=true`
- 禁用 HTTP: `SECURITY_URL_ALLOWLIST_ALLOW_INSECURE_HTTP=false`
- 配置允许的上游主机

## 功能特性

- ✅ 多账号管理 (OAuth, API Key)
- ✅ API Key 分发
- ✅ Token 级别计费
- ✅ 智能调度与粘性会话
- ✅ 并发控制
- ✅ 速率限制
- ✅ 管理后台
- ✅ Antigravity 支持

## 故障排查

### 查看容器状态
```bash
docker-compose -f docker-compose.local.yml ps
```

### 查看健康检查
```bash
docker inspect sub2api | grep -A 10 Health
```

### 数据库连接测试
```bash
docker exec -it sub2api-postgres psql -U sub2api -d sub2api -c "SELECT version();"
```

### Redis 连接测试
```bash
docker exec -it sub2api-redis redis-cli ping
```

## 注意事项

1. **端口冲突**: 已配置使用 8090 端口避免与 CLIProxyAPI 冲突
2. **数据持久化**: 使用本地目录映射，数据存储在 `deploy/` 下
3. **安全凭证**: 已生成的密钥请妥善保管，不要提交到版本控制
4. **时区设置**: 默认使用 `Asia/Shanghai`
5. **日志管理**: 日志自动轮转，保留 7 天，单文件最大 100MB

## 相关链接

- 项目仓库: https://github.com/nineton-bot/sub2api
- 上游项目: https://github.com/Wei-Shaw/sub2api
- 在线演示: https://demo.sub2api.org/
