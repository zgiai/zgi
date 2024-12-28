/** @type {import('next').NextConfig} */
const nextConfig = {
  webpack: (config) => {
    config.module.rules.push({
      test: /\.svg$/,
      use: ['@svgr/webpack'],
    });
    return config
  },
  rewrites: async () => {
    const ret = [
      {
        source: '/_internal/:path*',
        destination: `http://127.0.0.1:3000/api/health`,
      },
      {
        source: '/google-fonts/:path*',
        destination: 'https://fonts.googleapis.com/:path*',
      },
      {
        source: '/api/:path*',
        destination: 'https://bisheng.dataelem.com/api/:path*',
      },
    ];

    return {
      beforeFiles: ret,
    };
  },

}

module.exports = nextConfig
