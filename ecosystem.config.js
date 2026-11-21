const fs = require("fs");
const path = require("path");

// Load environment variables from .env
const env = {
  PORT: "8080",
  DB_PATH: "/data/data/com.termux/files/home/go-app/data.db",
  GIN_MODE: "release",
  APP_BASE_URL: process.env.APP_BASE_URL || "http://100.92.194.87:8080"
};

const envPath = path.resolve(__dirname, ".env");
if (fs.existsSync(envPath)) {
  try {
    const content = fs.readFileSync(envPath, "utf8");
    const lines = content.split("\n");
    for (const line of lines) {
      const match = line.match(/^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*)?$/);
      if (match) {
        const key = match[1];
        let val = (match[2] || "").trim().replace(/^['"]|['"]$/g, "");
        env[key] = val;
      }
    }
  } catch (err) {
    console.error("Failed to read .env file:", err.message);
  }
}

const apps = [
  {
    name: "go-server",
    script: "./server",
    cwd: "/data/data/com.termux/files/home/go-app",
    interpreter: "none",
    autorestart: true,
    watch: false,
    max_memory_restart: "250M",
    env: {
      ...env
    }
  }
];

module.exports = { apps };
