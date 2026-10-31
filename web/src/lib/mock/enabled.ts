// Mock switch. A public build-time variable (only the NEXT_PUBLIC_ prefix is readable in the browser).
// Setting NEXT_PUBLIC_MOCK=1 on Vercel sends the whole site through mocks, no backend needed.
export const MOCK = process.env.NEXT_PUBLIC_MOCK === "1";
