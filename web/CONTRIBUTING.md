# Contributing to Llamacto Web Scaffold

We love your input! We want to make contributing to Llamacto Web Scaffold as easy and transparent as possible, whether it's:

- Reporting a bug
- Discussing the current state of the code
- Submitting a fix
- Proposing new features
- Becoming a maintainer

## Development Process

We use GitHub to host code, to track issues and feature requests, as well as accept pull requests.

### Pull Requests

Pull requests are the best way to propose changes to the codebase. We actively welcome your pull requests:

1. Fork the repo and create your branch from `main`.
2. If you've added code that should be tested, add tests.
3. If you've changed APIs, update the documentation.
4. Ensure the test suite passes.
5. Make sure your code lints.
6. Issue that pull request!

### Development Setup

1. **Clone the repository**

   ```bash
   git clone https://github.com/yourusername/llamacto-web.git
   cd llamacto-web
   ```

2. **Install dependencies**

   ```bash
   pnpm install
   ```

3. **Start the development server**

   ```bash
   pnpm dev
   ```

4. **Open your browser**
   Navigate to [http://localhost:3000](http://localhost:3000)

### Code Style

We use several tools to maintain code quality and consistency:

- **ESLint**: For code linting
- **Prettier**: For code formatting
- **TypeScript**: For type safety

Run these commands before submitting your PR:

```bash
# Lint your code
pnpm lint

# Fix linting issues
pnpm lint:fix

# Format code
pnpm format

# Type check
pnpm type-check
```

### Commit Messages

We use conventional commit messages:

- `feat:` - A new feature
- `fix:` - A bug fix
- `docs:` - Documentation only changes
- `style:` - Changes that do not affect the meaning of the code
- `refactor:` - A code change that neither fixes a bug nor adds a feature
- `test:` - Adding missing tests or correcting existing tests
- `chore:` - Changes to the build process or auxiliary tools

Example:

```
feat: add user authentication system
fix: resolve navigation bug in mobile view
docs: update API documentation
```

### Branch Naming

Use descriptive branch names:

- `feature/user-authentication`
- `fix/navigation-bug`
- `docs/api-update`
- `refactor/state-management`

## Any contributions you make will be under the MIT Software License

In short, when you submit code changes, your submissions are understood to be under the same [MIT License](http://choosealicense.com/licenses/mit/) that covers the project. Feel free to contact the maintainers if that's a concern.

## Report bugs using GitHub's [issue tracker](https://github.com/yourusername/llamacto-web/issues)

We use GitHub issues to track public bugs. Report a bug by [opening a new issue](https://github.com/yourusername/llamacto-web/issues/new).

### Write bug reports with detail, background, and sample code

**Great Bug Reports** tend to have:

- A quick summary and/or background
- Steps to reproduce
  - Be specific!
  - Give sample code if you can
- What you expected would happen
- What actually happens
- Notes (possibly including why you think this might be happening, or stuff you tried that didn't work)

### Example Bug Report

```
**Quick Summary**: The login form doesn't submit when using Enter key

**Steps to reproduce**:
1. Navigate to /login
2. Enter valid credentials
3. Press Enter key instead of clicking Submit button
4. Nothing happens

**Expected behavior**: Form should submit and redirect to dashboard

**Actual behavior**: Form doesn't submit, no error message shown

**Environment**:
- Browser: Chrome 91.0.4472.124
- OS: macOS Big Sur 11.4
- Node.js: 16.14.0
```

## Feature Requests

We love feature requests! But please understand that we can't implement everything. When requesting a feature:

1. **Check existing issues** first to avoid duplicates
2. **Provide context** about your use case
3. **Describe the solution** you'd like
4. **Consider alternatives** you've thought of
5. **Additional context** that might be helpful

## Development Guidelines

### File Organization

```
src/
├── app/                    # Next.js App Router
│   ├── (auth)/            # Authentication pages
│   ├── (site)/            # Public pages
│   └── console/           # Admin dashboard
├── components/            # Reusable components
│   └── ui/               # UI component library
├── hooks/                # Custom React hooks
├── lib/                  # Utility functions
├── providers/            # React context providers
├── store/                # Zustand stores
└── utils/                # Helper utilities
```

### Component Guidelines

1. **Use TypeScript** for all components
2. **Follow naming conventions**: PascalCase for components, camelCase for functions
3. **Export components as default** when there's only one per file
4. **Use interfaces** for prop types
5. **Add JSDoc comments** for complex components

### Testing Guidelines

1. **Write tests** for new features
2. **Update tests** when modifying existing code
3. **Use descriptive test names**
4. **Test both happy path and error cases**

### Performance Guidelines

1. **Use dynamic imports** for large components
2. **Optimize images** with Next.js Image component
3. **Implement proper caching** strategies
4. **Monitor bundle size** with build analyzer

## Code of Conduct

### Our Pledge

We pledge to make participation in our project a harassment-free experience for everyone, regardless of age, body size, disability, ethnicity, gender identity and expression, level of experience, nationality, personal appearance, race, religion, or sexual identity and orientation.

### Our Standards

Examples of behavior that contributes to creating a positive environment include:

- Using welcoming and inclusive language
- Being respectful of differing viewpoints and experiences
- Gracefully accepting constructive criticism
- Focusing on what is best for the community
- Showing empathy towards other community members

## Getting Help

- **Documentation**: Check our [docs](https://docs.llamacto.com)
- **Discord**: Join our [community](https://discord.gg/llamacto)
- **Issues**: Create a [GitHub issue](https://github.com/yourusername/llamacto-web/issues)
- **Email**: Contact us at dev@llamacto.com

## License

By contributing, you agree that your contributions will be licensed under the MIT License.

## References

This document was adapted from the open-source contribution guidelines for [Facebook's Draft](https://github.com/facebook/draft-js/blob/a9316a723f9e918afde44dea68b5f9f39b7d9b00/CONTRIBUTING.md).

---

Thank you for contributing to Llamacto Web Scaffold! 🚀
