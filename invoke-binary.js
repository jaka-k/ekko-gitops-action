const childProcess = require('child_process')
const os = require('os')
const process = require('process')

const VERSION = '67341bfaa441ed8a69657030837c2e73c8433736'

function chooseBinary() {
    const platform = os.platform()
    const arch = os.arch()

    if (platform === 'linux' && arch === 'x64') {
        return `main-linux-amd64-${VERSION}`
    }
    if (platform === 'linux' && arch === 'arm64') {
        return `main-linux-arm64-${VERSION}`
    }
    if (platform === 'darwin' && arch === 'x64') {
        return `main-darwin-amd64-${VERSION}`
    }
    if (platform === 'darwin' && arch === 'arm64') {
        return `main-darwin-arm64-${VERSION}`
    }
    if (platform === 'windows' && arch === 'x64') {
        return `main-windows-amd64-${VERSION}`
    }
    if (platform === 'windows' && arch === 'arm64') {
        return `main-windows-arm64-${VERSION}`
    }

    console.error(`Unsupported platform (${platform}) and architecture (${arch})`)
    process.exit(1)
}

function main() {
    // The subcommand for the Go binary: CLI arg for local testing
    // (node invoke-binary.js dump-context), INPUT_COMMAND on a runner
    // (set automatically from the action's `command` input).
    const command = process.argv[2] || process.env.INPUT_COMMAND
    if (!command) {
        console.error('No command given (arg or INPUT_COMMAND)')
        process.exit(1)
    }

    const binary = chooseBinary()
    const mainScript = `${__dirname}/bin/${binary}`
    const spawnSyncReturns = childProcess.spawnSync(mainScript, [command], { stdio: 'inherit' })
    const status = spawnSyncReturns.status
    if (typeof status === 'number') {
        process.exit(status)
    }
    process.exit(1)
}

if (require.main === module) {
    main()
}