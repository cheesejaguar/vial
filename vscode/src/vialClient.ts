import { execFile } from 'child_process';
import { spawn } from 'child_process';

export interface RunResult {
	stdout: string;
	stderr: string;
	exitCode: number;
}

export class VialClient {
	constructor(private binaryPath: string) {}

	run(args: string[]): Promise<RunResult> {
		return new Promise((resolve) => {
			execFile(this.binaryPath, args, { timeout: 30000 }, (error, stdout, stderr) => {
				resolve({
					stdout: stdout || '',
					stderr: stderr || '',
					exitCode: error ? (error as any).code || 1 : 0,
				});
			});
		});
	}

	runWithStdin(args: string[], stdin: string): Promise<RunResult> {
		return new Promise((resolve) => {
			const proc = spawn(this.binaryPath, args, { timeout: 30000 });
			let stdout = '';
			let stderr = '';

			proc.stdout.on('data', (data: Buffer) => { stdout += data.toString(); });
			proc.stderr.on('data', (data: Buffer) => { stderr += data.toString(); });

			proc.on('close', (code: number | null) => {
				resolve({ stdout, stderr, exitCode: code || 0 });
			});

			proc.stdin.write(stdin + '\n');
			proc.stdin.end();
		});
	}
}
