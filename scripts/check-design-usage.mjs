import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import process from 'node:process';
import console from 'node:console';
import { checkTokenUsage, declaredDesignTokens } from '../src/design/usage.ts';

const root = resolve(import.meta.dirname, '..');
const files = [
  'src/client/ui/ui.css',
  'src/client/styles/design-alignment.css',
  'src/client/styles/desktop-design-alignment.css',
  'src/client/desktop/assistant-markdown.css',
  'src/client/desktop/turn-process.css',
  'src/client/desktop/conversation-fixture.css',
  'src/client/desktop/trace.css',
  'src/client/desktop/style.css',
  'src/client/desktop/resource-page.css',
  'src/client/desktop/collaboration-page.css',
  'src/client/desktop/channel-page.css',
  'src/client/desktop/runtime-manager.css',
  'src/client/desktop/schedule-page.css',
];
const failures = [];
const declared = declaredDesignTokens(
  readFileSync(resolve(root, 'src/client/styles/design-tokens.css'), 'utf8'),
);
for (const file of files) {
  const content = readFileSync(resolve(root, file), 'utf8');
  failures.push(
    ...checkTokenUsage(content, declared).map(
      (failure) => `${file}: ${failure}`,
    ),
  );
}

const entrypointImports = {
  'src/client/main.tsx': ["import './style.css'", "import './editor.css'"],
  'src/client/desktop/main.tsx': [
    "import '../style.css'",
    "import '../editor.css'",
    "import './style.css'",
  ],
};
for (const [file, legacyImports] of Object.entries(entrypointImports)) {
  const content = readFileSync(resolve(root, file), 'utf8');
  const alignment = content.indexOf('styles/design-alignment.css');
  if (alignment < 0) {
    failures.push(`${file} does not import the design alignment layer`);
    continue;
  }
  for (const statement of legacyImports) {
    const legacy = content.indexOf(statement);
    if (legacy < 0)
      failures.push(`${file} is missing expected style import: ${statement}`);
    else if (legacy > alignment)
      failures.push(
        `${file} loads ${statement} after the design alignment layer`,
      );
  }
}
if (failures.length > 0) {
  console.error(failures.join('\n'));
  process.exit(1);
}
console.log(
  `Design token usage is clean in ${files.length} migrated stylesheets.`,
);
