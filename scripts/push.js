const { execSync } = require('child_process');
const readline = require('readline');

function run(cmd, capture = false) {
  try {
    if (capture) {
      return execSync(cmd, { encoding: 'utf8' }).trim();
    } else {
      return execSync(cmd, { stdio: 'inherit' });
    }
  } catch (err) {
    process.exit(1);
  }
}

function hasChanges() {
  const status = run('git status --porcelain', true);
  return status.length > 0;
}

if (!hasChanges()) {
  console.log('No uncommitted changes detected. Pushing current branch...');
  run('git push');
  process.exit(0);
}

const rl = readline.createInterface({
  input: process.stdin,
  output: process.stdout
});

rl.question('Enter commit message: ', (message) => {
  rl.close();
  if (!message.trim()) {
    console.error('Commit message cannot be empty.');
    process.exit(1);
  }

  console.log('Staging changes...');
  run('git add .');

  console.log(`Committing with message: "${message}"...`);
  const escapedMessage = message.replace(/"/g, '\\"');
  run(`git commit -m "${escapedMessage}"`);

  console.log('Pushing code to remote repository...');
  run('git push');

  console.log('Successfully pushed!');
});
