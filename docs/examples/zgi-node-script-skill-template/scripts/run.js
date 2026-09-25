#!/usr/bin/env node

const { main } = require("./zgi_skill_node_template");

main().then(
  (code) => {
    process.exitCode = code;
  },
  (error) => {
    process.stderr.write(`script input error: ${error.message}\n`);
    process.exitCode = 2;
  },
);
