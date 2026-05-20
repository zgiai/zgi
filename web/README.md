# 🚀 ZGI Web Platform

A production-ready **AI workflow platform** built with Next.js, enabling users to create, manage, and execute AI-powered workflows with real-time streaming capabilities.

## ✨ Features

- 🤖 **AI Workflow Engine**: Visual workflow editor with drag-and-drop node-based design
- 🔄 **Real-time Streaming**: SSE (Server-Sent Events) for live workflow execution updates
- 🎯 **Modern Stack**: Built with Next.js 16, React 19, TypeScript, and Tailwind CSS
- 🔐 **Authentication**: Complete auth system with login, registration, and protected routes
- 📊 **Admin Console**: Full-featured admin interface with analytics and data visualization
- 🎨 **UI Components**: Extensive component library with Radix UI (shadcn/ui)
- 🌙 **Theme System**: Multiple themes (Light, Dark, AI Young) with automatic system detection
- 🌍 **Internationalization**: Full i18n support with 21 translation modules (en-US, zh-Hans)
- 📱 **Responsive Design**: Mobile-first approach with responsive layouts
- 🔍 **TypeScript**: Strict type safety with no `any` types
- 🎭 **State Management**: Hybrid approach with Zustand (global) + TanStack Query (server)
- 📝 **Rich Editors**: Monaco Editor for code, Tiptap for rich text
- 📦 **Production Ready**: Docker, PM2, and cloud deployment support
- 🔬 **Error Tracking**: Sentry integration for production monitoring

## 🛠️ Tech Stack

| Category | Technology | Version |
|----------|------------|---------|
| **Framework** | Next.js (App Router) | 16.1.6 |
| **UI Library** | React | 19.2.1 |
| **Language** | TypeScript | 5.x |
| **Styling** | Tailwind CSS | 3.4.1 |
| **UI Components** | Radix UI (shadcn/ui) | Latest |
| **State Management** | Zustand | 5.0.2 |
| **Server State** | TanStack Query | 5.80.7 |
| **Internationalization** | next-intl | 4.1.2 |
| **Workflow Editor** | @xyflow/react | 12.8.4 |
| **Code Editor** | Monaco Editor | 0.53.0 |
| **Rich Text Editor** | Tiptap | 3.4.4 |
| **Drag & Drop** | @dnd-kit | Latest |
| **Forms** | React Hook Form + Zod | Latest |
| **Icons** | Lucide React | Latest |
| **Charts** | Recharts | 2.15.3 |
| **Error Tracking** | Sentry | 10.29.0 |
| **Package Manager** | pnpm | 10.12.1 |

## 🚀 Quick Start

### Prerequisites

- Node.js 18+ 
- pnpm (recommended) or npm

### Installation

1. **Clone the repository**
   ```bash
   git clone https://github.com/yourusername/zgi-web-next.git
   cd zgi-web-next
   ```

2. **Install dependencies**
   ```bash
   pnpm install
   ```

3. **Set up environment variables**
   ```bash
   cp .env.example .env
   # Edit .env with your API endpoints and configuration
   ```

4. **Run the development server**
   ```bash
   pnpm dev
   ```

5. **Open your browser**
   Navigate to [http://localhost:3000](http://localhost:3000)

## 🔐 Authentication

Pre-built authentication system includes:

- **Login/Register**: Email and password authentication
- **Protected Routes**: Route-level protection with middleware
- **User Management**: User profiles and settings
- **Password Reset**: Secure password recovery
- **Session Management**: JWT-based sessions with auto-refresh
- **Multi-tenant**: Organization and workspace context

## 🤖 AI Workflow System

The core feature of ZGI - a visual workflow editor for AI operations:

### Visual Workflow Editor
- **Node-based Design**: Built with `@xyflow/react`
- **Drag & Drop**: Intuitive node manipulation
- **Custom Node Types**: LLM, HTTP, Condition, Transform, etc.
- **Connection Validation**: Data flow management

### Workflow Execution
- **Draft Mode**: Test execution without saving
- **Production Mode**: Saved workflow execution
- **Streaming Execution**: Real-time SSE for live updates
- **HTTP Nodes**: REST API integration within workflows

## 📊 Admin Console

Full-featured admin interface with:

- **Analytics Dashboard**: Charts and metrics with Recharts
- **Agent Management**: Create and manage AI agents
- **Dataset Management**: Upload and manage training datasets
- **Database Management**: SQL operations and data exploration
- **User Management**: CRUD operations for users
- **Settings Panel**: Application configuration
- **Data Tables**: Sortable, filterable tables with TanStack Table

## 🚀 Deployment

### Docker (Recommended for Self-Hosted)

The easiest way to deploy ZGI Web is using Docker:

```bash
# 1. Copy and configure environment file
cp .env.example .env
# Edit .env with your API URL and other settings

# 2. Build the image
docker build -t zgi-web:latest .

# 3. Run the container
docker run -d \
  --name zgi-web \
  -p 3000:3000 \
  --env-file .env \
  --restart unless-stopped \
  zgi-web:latest

# Or start with Docker Compose
docker compose --env-file .env -f docker-compose.yml up -d --build
```

**Environment Files:**
- `.env.example` - Unified template for both development and Docker deployment
- `.env` - Local runtime file used by `docker run` / `docker compose`
- `BASE_PATH` / `NEXT_PUBLIC_BASE_PATH` - Shared basePath variables. Docker Compose now resolves a single value for build/runtime (`BASE_PATH` first, then fallback to `NEXT_PUBLIC_BASE_PATH`).
- Prefer `NEXT_PUBLIC_*` keys in `.env`; entrypoint keeps only minimal legacy aliases for compatibility.

**BasePath Note:**
- `docker compose --env-file .env ...` will pass `.env` values to both build-time and runtime.
- If you use `docker build` directly and need sub-route deployment, pass build arg explicitly:
  `docker build --build-arg BASE_PATH=/zgi --build-arg NEXT_PUBLIC_BASE_PATH=/zgi -t zgi-web:latest .`

**Features:**
- ✅ Zero-downtime deployment with health checks
- ✅ Automatic restart on failure
- ✅ Built-in PM2 cluster mode (configurable via PM2_INSTANCES)
- ✅ Production-optimized Next.js standalone build
- ✅ Flexible environment injection for deployment configuration

### PM2 (Alternative for Node.js Environments)

For direct Node.js deployment:

```bash
# Install dependencies and build
pnpm install --frozen-lockfile
pnpm build

# Copy static files for standalone mode
cp -r .next/static .next/standalone/.next/static
cp -r public .next/standalone/public

# Start with PM2
pm2 start ecosystem.config.js --env production
```

### Cloud Platforms

ZGI can be deployed to:

- **Vercel**: Zero-config deployment for Next.js
- **AWS ECS/Fargate**: Container orchestration
- **Google Cloud Run**: Serverless containers
- **Azure Container Instances**: Managed containers
- **Railway/Render**: Simple PaaS deployment

📖 **Detailed deployment guide**: See [DEPLOYMENT_GUIDE.md](DEPLOYMENT_GUIDE.md) for complete instructions, zero-downtime strategies, and troubleshooting.

## 🧪 Scripts

```bash
# Development
pnpm dev          # Start development server (Turbopack)
pnpm build        # Build for production
pnpm start        # Start production server

# PM2 Management
pnpm pm2:start    # Start with PM2
pnpm pm2:stop     # Stop PM2 process
pnpm pm2:restart  # Restart PM2 process
pnpm pm2:logs     # View PM2 logs
pnpm pm2:status   # Check PM2 status

# Code Quality
pnpm lint         # Run ESLint
pnpm lint:fix     # Fix ESLint errors
pnpm type-check   # Run TypeScript checks
pnpm format       # Format code with Prettier
```

## 🤝 Contributing

We welcome contributions! Please see our [Contributing Guide](CONTRIBUTING.md) for details.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

- [Next.js](https://nextjs.org/) - The React framework for production
- [React](https://react.dev/) - UI library
- [Tailwind CSS](https://tailwindcss.com/) - A utility-first CSS framework
- [Radix UI](https://www.radix-ui.com/) - Low-level UI primitives
- [Zustand](https://zustand-demo.pmnd.rs/) - State management
- [TanStack Query](https://tanstack.com/query) - Data fetching and caching
- [XYFlow](https://xyflow.com/) - Workflow editor
- [Monaco Editor](https://microsoft.github.io/monaco-editor/) - Code editor
- [Tiptap](https://tiptap.dev/) - Rich text editor
- [Sentry](https://sentry.io/) - Error tracking

## 📞 Support

- 📧 Email: support@zgi.ai
- 🌐 Website: [zgi.ai](https://zgi.ai)
- 🐛 Issues: [GitHub Issues](https://github.com/zgiai/zgi-web-next/issues)

---

Made with ❤️ by the ZGI Team

