<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { EventsOn, EventsOff } from "../wailsjs/runtime/runtime.js";
  import { ProcessFile, GetOCRProvider } from "../wailsjs/go/app/App.js";

  import FileUpload from "./lib/FileUpload.svelte";
  import ProcessingView from "./lib/ProcessingView.svelte";
  import ResultsView from "./lib/ResultsView.svelte";

  import type { ProcessingProgress, ProcessingResult } from "./types.js";

  type Step = "setup" | "processing" | "done";

  let step: Step = "setup";
  let inputPath = "";
  let outputDir = "";
  let progress: ProcessingProgress | null = null;
  let error: string | null = null;
  let result: ProcessingResult | null = null;
  let ocrProvider = "";
  let whitelistText = "";

  function startProcessing() {
    step = "processing";
    progress = null;
    error = null;
    result = null;
    ProcessFile(inputPath, outputDir, whitelistText).catch((err: unknown) => {
      error = err instanceof Error ? err.message : String(err);
    });
  }

  function reset() {
    step = "setup";
    inputPath = "";
    outputDir = "";
    progress = null;
    error = null;
    result = null;
    whitelistText = "";
  }

  onMount(() => {
    GetOCRProvider().then((v) => (ocrProvider = v));

    EventsOn("processing:progress", (p: ProcessingProgress) => {
      progress = p;
    });

    EventsOn("processing:complete", (r: ProcessingResult) => {
      result = r;
      step = "done";
    });

    EventsOn("processing:error", (msg: string) => {
      error = msg;
    });
  });

  onDestroy(() => {
    EventsOff("processing:progress");
    EventsOff("processing:complete");
    EventsOff("processing:error");
  });
</script>

<div class="layout">
  <header class="topbar">
    <span class="app-name">ScanSplit</span>
    {#if ocrProvider}
      <span class="ocr-badge" class:mock={ocrProvider.startsWith("Mock")}>OCR: {ocrProvider}</span>
    {/if}
    {#if step !== "setup"}
      <div class="breadcrumb">
        <!-- svelte-ignore a11y-no-static-element-interactions -->
        <span
          class="crumb"
          class:active={step === "setup"}
          on:click={step === "processing" ? undefined : reset}
        >Файл</span>
        <span class="sep">›</span>
        <span class="crumb" class:active={step === "processing"}>Обработка</span>
        {#if step === "done"}
          <span class="sep">›</span>
          <span class="crumb active">Результат</span>
        {/if}
      </div>
    {/if}
  </header>

  <main class="content">
    {#if step === "setup"}
      <FileUpload
        bind:inputPath
        bind:outputDir
        bind:whitelistText
        onReady={startProcessing}
      />
    {:else if step === "processing"}
      <ProcessingView {progress} {error} onCancel={reset} />
    {:else if step === "done" && result}
      <ResultsView {result} {outputDir} onReset={reset} />
    {/if}
  </main>
</div>

<style>
  .layout {
    display: flex;
    flex-direction: column;
    height: 100%;
  }

  .topbar {
    display: flex;
    align-items: center;
    gap: 16px;
    padding: 0 20px;
    height: 44px;
    background: var(--bg-surface);
    border-bottom: 1px solid var(--border);
    flex-shrink: 0;
    /* Wails drag region */
    --wails-draggable: drag;
  }

  .app-name {
    font-size: 13px;
    font-weight: 600;
    color: var(--text);
    letter-spacing: 0.02em;
  }

  .breadcrumb {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
    color: var(--text-muted);
  }

  .crumb { cursor: default; }
  .crumb.active { color: var(--text); }

  .sep { color: var(--border); }

  .ocr-badge {
    font-size: 10px;
    font-weight: 500;
    padding: 2px 7px;
    border-radius: 10px;
    background: color-mix(in srgb, var(--accent) 15%, transparent);
    color: var(--accent);
    letter-spacing: 0.02em;
  }
  .ocr-badge.mock {
    background: color-mix(in srgb, #f59e0b 15%, transparent);
    color: #f59e0b;
  }

  .content {
    flex: 1;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    align-items: center;
    padding-top: 12px;
  }
</style>
