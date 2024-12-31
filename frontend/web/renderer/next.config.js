/** @type {import('next').NextConfig} */
module.exports = {
  // output: "export",
  distDir: process.env.NODE_ENV === 'production' ? '../app' : '.next',
  trailingSlash: true,
  images: {
    unoptimized: true,
  },
  webpack: (config) => {
    return config
  },
  async rewrites() {
    return [
      {
        source: '/api/:path*',
        destination: 'https://zgi.zeabur.app/:path*',
      },
    ]
  },
}
