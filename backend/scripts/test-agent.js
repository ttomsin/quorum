// test-agent.js
//
// Lets Gemini reason over the exact same 5 tools Quorum registers via
// WebMCP, then calls whichever one it picks against your running server.
//
// Usage:
//   export GEMINI_API_KEY=your_key_here
//   node test-agent.js "Vote for Nairobi on behalf of Ada's Agent"
//
// Requires the board server running first: `go run main.go` (in the repo root)

const { GoogleGenerativeAI } = require('@google/generative-ai');

const API_BASE = 'http://localhost:8080';

if (!process.env.GEMINI_API_KEY) {
    console.error('Set GEMINI_API_KEY as an environment variable first.');
    process.exit(1);
}

const prompt = process.argv.slice(2).join(' ');
if (!prompt) {
    console.error('Usage: node test-agent.js "your instruction here"');
    process.exit(1);
}

// Same shape as the tools registered in static/index.html via
// document.modelContext.registerTool()
const tools = [
    {
        functionDeclarations: [
            {
                name: 'get_board_state',
                description: 'Get the current deliberation board: topic, participants, options, arguments, and votes.',
                parameters: { type: 'object', properties: {} },
            },
            {
                name: 'propose_option',
                description: 'Propose a new option on the deliberation board for the group to consider.',
                parameters: {
                    type: 'object',
                    properties: {
                        text: { type: 'string', description: 'The option being proposed' },
                        participant_id: { type: 'string', description: 'ID of the participant proposing it' },
                    },
                    required: ['text', 'participant_id'],
                },
            },
            {
                name: 'argue_for',
                description: 'Post a short argument in favor of an existing option on the board.',
                parameters: {
                    type: 'object',
                    properties: {
                        option_id: { type: 'string', description: 'ID of the option being argued for' },
                        participant_id: { type: 'string', description: 'ID of the participant making the argument' },
                        text: { type: 'string', description: 'The argument text' },
                    },
                    required: ['option_id', 'participant_id', 'text'],
                },
            },
            {
                name: 'cast_vote',
                description: "Cast or update this participant's vote for an option on the board.",
                parameters: {
                    type: 'object',
                    properties: {
                        option_id: { type: 'string', description: 'ID of the option to vote for' },
                        participant_id: { type: 'string', description: 'ID of the participant voting' },
                    },
                    required: ['option_id', 'participant_id'],
                },
            },
            {
                name: 'override_vote',
                description: "Human-only: override the vote your own agent cast on your behalf.",
                parameters: {
                    type: 'object',
                    properties: {
                        option_id: { type: 'string', description: 'ID of the option to switch the vote to' },
                        participant_id: { type: 'string', description: 'ID of the human participant overriding their agent' },
                    },
                    required: ['option_id', 'participant_id'],
                },
            },
        ],
    },
];

// Actually execute a tool call against the real running server --
// same endpoints the browser UI and WebMCP tools call.
async function executeTool(name, args) {
    switch (name) {
        case 'get_board_state': {
            const res = await fetch(`${API_BASE}/api/board`);
            return res.json();
        }
        case 'propose_option': {
            const res = await fetch(`${API_BASE}/api/options`, {
                method: 'POST',
                body: JSON.stringify(args),
            });
            return res.json();
        }
        case 'argue_for': {
            const res = await fetch(`${API_BASE}/api/arguments`, {
                method: 'POST',
                body: JSON.stringify(args),
            });
            return res.json();
        }
        case 'cast_vote':
        case 'override_vote': {
            const res = await fetch(`${API_BASE}/api/vote`, {
                method: 'POST',
                body: JSON.stringify(args),
            });
            return res.json();
        }
        default:
            throw new Error(`Unknown tool: ${name}`);
    }
}

async function main() {
    const genAI = new GoogleGenerativeAI(process.env.GEMINI_API_KEY);
    const model = genAI.getGenerativeModel({ model: 'gemini-3.6-flash', tools });

    // Give the model the current board state up front so it can reason
    // about real option IDs instead of guessing them.
    const board = await (await fetch(`${API_BASE}/api/board`)).json();

    const chat = model.startChat();
    const result = await chat.sendMessage(
        `Current board state: ${JSON.stringify(board)}\n\nInstruction: ${prompt}`
    );

    const calls = result.response.functionCalls();
    if (!calls || calls.length === 0) {
        console.log('Gemini did not call a tool. Response:', result.response.text());
        return;
    }

    for (const call of calls) {
        console.log(`\n→ Gemini called: ${call.name}(${JSON.stringify(call.args)})`);
        const toolResult = await executeTool(call.name, call.args);
        console.log('→ Result:', JSON.stringify(toolResult, null, 2));
    }
}

main().catch((err) => {
    console.error('Error:', err.message);
    process.exit(1);
});