// 复刻 quality-gate.ts 的 cmdExeReachable()，在真实环境验证判定结果
const { accessSync, constants } = require('node:fs');
const { join, delimiter } = require('node:path');

function cmdExeReachable() {
	if (process.platform === 'win32') return true;
	for (const dir of (process.env.PATH ?? '').split(delimiter)) {
		if (!dir) continue;
		for (const name of ['cmd.exe', 'cmd']) {
			try {
				accessSync(join(dir, name), constants.X_OK);
				return true;
			} catch {
				/* 继续找 */
			}
		}
	}
	return false;
}

const reachable = cmdExeReachable();
console.log('process.platform      :', process.platform);
console.log('cmdExeReachable()     :', reachable);
console.log('=> 判定模式           :', reachable ? 'windows (cmd.exe interop)' : 'native (本机工具链)');
console.log('本机 go               :', require('node:child_process').execSync('command -v go').toString().trim());
console.log('本机 golangci-lint    :', require('node:child_process').execSync('command -v golangci-lint').toString().trim());
console.log('本机 pnpm             :', require('node:child_process').execSync('command -v pnpm').toString().trim());
