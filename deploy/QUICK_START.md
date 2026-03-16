# Sub2API 快速开始

## 🎉 部署成功！

Sub2API 已成功部署并运行在您的系统上。

## 📋 访问信息

### Web 管理界面
- **地址**: http://localhost:8090
- **管理员邮箱**: admin@sub2api.local
- **管理员密码**: `8bfdadc5ca709b5e9962908525b45690`

⚠️ **重要**: 首次登录后请立即修改密码！

### API 端点
- **基础 URL**: http://localhost:8090/v1/
- **健康检查**: http://localhost:8090/health

## 🚀 服务状态

当前运行的容器：
- `sub2api` - 主应用服务 (端口 8090)
- `sub2api-postgres` - PostgreSQL 数据库
- `sub2api-redis` - Redis 缓存

## 📊 常用命令

```bash
# 进入部署目录
cd /Users/lsfhost/.openclaw/agents/proxymaster/workspace/sub2api/deploy

# 查看服务状态
docker-compose -f docker-compose.local.yml ps

# 查看实时日志
docker-compose -f docker-compose.local.yml logs -f sub2api

# 重启服务
docker-compose -f docker-compose.local.yml restart

# 停止服务
docker-compose -f docker-compose.local.yml down

# 启动服务
docker-compose -f docker-compose.local.yml up -d
```

## 🔧 下一步操作

1. **登录管理后台**
   - 访问 http://localhost:8090
   - 使用上述管理员账号登录
   - 修改默认密码

2. **添加上游账号**
   - 在管理后台添加 AI 服务账号（Claude、Gemini 等）
   - 支持 OAuth 和 API Key 两种方式

3. **创建 API Key**
   - 为用户生成 API Key
   - 配置配额和速率限制

4. **配置客户端**
   ```bash
   # 示例：配置 Claude Code
   export ANTHROPIC_BASE_URL="http://localhost:8090"
   export ANTHROPIC_AUTH_TOKEN="sk-your-api-key"
   ```

## 📚 功能特性

- ✅ 多账号管理 (OAuth, API Key)
- ✅ API Key 分发与管理
- ✅ Token 级别精确计费
- ✅ 智能调度与负载均衡
- ✅ 并发控制与速率限制
- ✅ 实时监控仪表盘
- ✅ 支持 Antigravity 账号

## 🔒 安全提示

当前配置为开发/测试模式：
- 允许 HTTP 连接
- 允许私有 IP 地址
- URL 白名单已禁用

**生产环境建议**：
1. 启用 HTTPS
2. 配置 URL 白名单
3. 限制私有 IP 访问
4. 定期备份数据

## 📖 完整文档

详细配置和使用说明请查看：
- `DEPLOYMENT_NOTES.md` - 完整部署文档
- `README.md` - 项目说明
- `README_CN.md` - 中文说明

## 🆘 故障排查

### 服务无法访问
```bash
# 检查容器状态
docker-compose -f docker-compose.local.yml ps

# 查看错误日志
docker-compose -f docker-compose.local.yml logs sub2api
```

### 端口冲突
如果 8090 端口被占用，修改 `.env` 文件：
```bash
SERVER_PORT=8091  # 改为其他端口
```
然后重启服务。

### 数据库连接问题
```bash
# 测试数据库连接
docker exec -it sub2api-postgres psql -U sub2api -d sub2api -c "SELECT version();"
```

## 📞 获取帮助

- GitHub Issues: https://github.com/nineton-bot/sub2api/issues
- 上游项目: https://github.com/Wei-Shaw/sub2api
- 在线演示: https://demo.sub2api.org/

---

**祝使用愉快！** 🎊
