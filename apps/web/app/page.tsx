const APIBaseURL = process.env.NEXT_PUBLIC_API_BASE_URL || "http://localhost:8080";

export default function HomePage() {
  return (
    <main className="shell">
      <section className="card">
        <p className="eyebrow">Vertex RAG</p>
        <h1>MVP scaffold is up</h1>
        <p>Frontend, API, worker, database, and compose baseline are ready.</p>
        <p className="hint">
          API base URL: <code>{APIBaseURL}</code>
        </p>
      </section>
    </main>
  );
}

