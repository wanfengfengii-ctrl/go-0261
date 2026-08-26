// Fixed build script: copies the single-page console into the Go embed target.
import { cp, mkdir } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const root = dirname(fileURLToPath(import.meta.url));
const dist = join(root, '..', 'webembed', 'dist');

await mkdir(dist, { recursive: true });
await cp(join(root, 'src', 'index.html'), join(dist, 'index.html'));
await cp(join(root, 'src', 'app.js'), join(dist, 'app.js'));
console.log('built frontend -> webembed/dist');
