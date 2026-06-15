const { execSync } = require('child_process');
const fs = require('fs');
const path = require('path');
const readline = require('readline');

function run(cmd, capture = false, allowFail = false) {
  try {
    if (capture) {
      return execSync(cmd, { encoding: 'utf8', stdio: ['pipe', 'pipe', 'ignore'] }).trim();
    } else {
      return execSync(cmd, { stdio: 'inherit' });
    }
  } catch (err) {
    if (allowFail) {
      throw err;
    }
    process.exit(1);
  }
}

// 1. Get latest tag or fallback to version in web/package.json
let latestTag = '';
try {
  latestTag = run('git describe --tags --abbrev=0', true, true);
} catch (e) {
  const webPkg = JSON.parse(fs.readFileSync(path.join(__dirname, '../web/package.json'), 'utf8'));
  latestTag = 'v' + webPkg.version;
}

console.log(`Latest version tag found: ${latestTag}`);

// 2. Parse and increment patch version
const versionMatch = latestTag.match(/^v?(\d+)\.(\d+)\.(\d+)$/);
if (!versionMatch) {
  console.error(`Invalid tag format: ${latestTag}. Must be vX.Y.Z`);
  process.exit(1);
}

const major = parseInt(versionMatch[1], 10);
const minor = parseInt(versionMatch[2], 10);
const patch = parseInt(versionMatch[3], 10);
const nextVersion = `${major}.${minor}.${patch + 1}`;
const nextTag = `v${nextVersion}`;

console.log(`Target next version: ${nextTag}`);

const rl = readline.createInterface({
  input: process.stdin,
  output: process.stdout
});

rl.question('Enter release/commit message: ', (message) => {
  rl.close();
  if (!message.trim()) {
    console.error('Commit message cannot be empty.');
    process.exit(1);
  }

  // 3. Update web/package.json
  const webPkgPath = path.join(__dirname, '../web/package.json');
  const webPkg = JSON.parse(fs.readFileSync(webPkgPath, 'utf8'));
  webPkg.version = nextVersion;
  fs.writeFileSync(webPkgPath, JSON.stringify(webPkg, null, 2) + '\n');

  // 4. Update wails.json
  const wailsJsonPath = path.join(__dirname, '../wails.json');
  const wailsJson = JSON.parse(fs.readFileSync(wailsJsonPath, 'utf8'));
  if (!wailsJson.info) {
    wailsJson.info = {};
  }
  wailsJson.info.productVersion = nextVersion;
  fs.writeFileSync(wailsJsonPath, JSON.stringify(wailsJson, null, 2) + '\n');

  console.log('Updated package.json and wails.json');

  // 5. Commit all changes (user changes + version bumps)
  console.log('Staging changes...');
  run('git add .');

  const commitMsg = `release: ${nextTag} - ${message}`;
  const escapedCommitMsg = commitMsg.replace(/"/g, '\\"');
  console.log(`Committing: "${commitMsg}"...`);
  run(`git commit -m "${escapedCommitMsg}"`);

  // 6. Push code
  console.log('Pushing code to remote repository...');
  run('git push');

  // 7. Tag and push tag
  console.log(`Tagging with ${nextTag} and pushing...`);
  run(`git tag ${nextTag}`);
  run(`git push origin ${nextTag}`);

  console.log(`Successfully bumped, committed, tagged, and pushed ${nextTag}!`);
});
