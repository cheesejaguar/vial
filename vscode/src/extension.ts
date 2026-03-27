import * as vscode from 'vscode';
import { VialClient } from './vialClient';

let statusBarItem: vscode.StatusBarItem;
let client: VialClient;

export function activate(context: vscode.ExtensionContext) {
	const config = vscode.workspace.getConfiguration('vial');
	const binaryPath = config.get<string>('binaryPath', 'vial');
	client = new VialClient(binaryPath);

	// Status bar item
	statusBarItem = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Right, 100);
	statusBarItem.command = 'vial.list';
	context.subscriptions.push(statusBarItem);
	updateStatusBar();

	// Register commands
	context.subscriptions.push(
		vscode.commands.registerCommand('vial.pour', async () => {
			const workspaceFolder = vscode.workspace.workspaceFolders?.[0];
			if (!workspaceFolder) {
				vscode.window.showErrorMessage('No workspace folder open');
				return;
			}

			const result = await client.run(['pour', '--dir', workspaceFolder.uri.fsPath]);
			if (result.exitCode === 0) {
				vscode.window.showInformationMessage('Vial: .env populated from vault');
			} else {
				vscode.window.showErrorMessage(`Vial pour failed: ${result.stderr}`);
			}
		}),

		vscode.commands.registerCommand('vial.diff', async () => {
			const workspaceFolder = vscode.workspace.workspaceFolders?.[0];
			if (!workspaceFolder) {
				vscode.window.showErrorMessage('No workspace folder open');
				return;
			}

			const result = await client.run(['diff', '--dir', workspaceFolder.uri.fsPath]);
			const channel = vscode.window.createOutputChannel('Vial Diff');
			channel.appendLine(result.stdout);
			if (result.stderr) {
				channel.appendLine(result.stderr);
			}
			channel.show();
		}),

		vscode.commands.registerCommand('vial.lock', async () => {
			const result = await client.run(['cork']);
			if (result.exitCode === 0) {
				vscode.window.showInformationMessage('Vial: Vault locked');
				updateStatusBar();
			} else {
				vscode.window.showErrorMessage(`Vial lock failed: ${result.stderr}`);
			}
		}),

		vscode.commands.registerCommand('vial.unlock', async () => {
			const password = await vscode.window.showInputBox({
				prompt: 'Enter vault master password',
				password: true
			});
			if (!password) { return; }

			const result = await client.runWithStdin(['uncork'], password);
			if (result.exitCode === 0) {
				vscode.window.showInformationMessage('Vial: Vault unlocked');
				updateStatusBar();
			} else {
				vscode.window.showErrorMessage(`Vial unlock failed: ${result.stderr}`);
			}
		}),

		vscode.commands.registerCommand('vial.list', async () => {
			const result = await client.run(['key', 'list']);
			const channel = vscode.window.createOutputChannel('Vial Secrets');
			channel.appendLine(result.stdout);
			channel.show();
		})
	);
}

function updateStatusBar() {
	client.run(['version']).then(result => {
		if (result.exitCode === 0) {
			statusBarItem.text = '$(key) Vial';
			statusBarItem.tooltip = result.stdout.trim();
			statusBarItem.show();
		}
	}).catch(() => {
		statusBarItem.hide();
	});
}

export function deactivate() {
	statusBarItem?.dispose();
}
