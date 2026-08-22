# Bat-ti — frontend

Phone-first React client for the raw material price and investment tracker.
Vite + TypeScript, TanStack Query for server state, Recharts for the graphs,
Tailwind for styling.

## Running it

The Go API must be running first (`make api` from the repository root, which
serves on `:8080`). Then:

```sh
npm install
npm run dev
```

`vite.config.ts` proxies `/api` to `http://localhost:8080`, so the app talks to
same-origin paths and no CORS handling is needed on either side. Point it
somewhere else with `VITE_API_TARGET`.

## Layout

```
src/
├── api/        types mirroring the Go response structs, the fetch wrapper,
│               and one TanStack Query hook per endpoint
├── components/ the app shell and the shared UI primitives
├── lib/        rupee, date and staleness formatting
└── pages/      one file per page of the product PRD (P1–P5)
```

Money arrives from the API as strings, not numbers — the backend encodes
`numeric` columns with `shopspring/decimal` so amounts are exact. They stay
strings here too, parsed only at the point of display.
