module.exports = {
  apps: [
    {
      name: 'zgi-web',
      script: 'server.js',
      cwd: process.cwd(),
      instances: process.env.INSTANCES || 1,
      exec_mode: 'cluster',
      watch: false,
      max_memory_restart: process.env.MAX_MEMORY || '1G',
      env: {
        NODE_ENV: 'production',
        PORT: process.env.PORT || 3000,
        HOSTNAME: '0.0.0.0'
      },
      error_file: './logs/error.log',
      out_file: './logs/out.log',
      log_file: './logs/combined.log',
      time: true
    }
  ]
};
