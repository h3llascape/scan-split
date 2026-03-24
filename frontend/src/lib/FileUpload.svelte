<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { EventsOn, EventsOff, OnFileDrop } from "../../wailsjs/runtime/runtime.js";
  import { SelectInputFile, SelectOutputDir } from "../../wailsjs/go/app/App.js";

  export let inputPath: string = "";
  export let outputDir: string = "";
  export let onReady: () => void;

  let dragging = false;
  let dropError = "";

  async function pickFile() {
    const path = await SelectInputFile();
    if (path) inputPath = path;
  }

  async function pickDir() {
    const dir = await SelectOutputDir();
    if (dir) outputDir = dir;
  }

  // Visual feedback only — actual path comes via Wails OnFileDrop event below.
  function handleDragOver() { dragging = true; }
  function handleDragLeave() { dragging = false; }
  function handleDrop()     { dragging = false; }

  function blockNativeDrop(e: Event) { e.preventDefault(); }

  onMount(() => {
    // Prevent WKWebView from opening dropped files as documents.
    document.addEventListener("dragover", blockNativeDrop);
    document.addEventListener("drop", blockNativeDrop);

    // Wails native file-drop: Go emits "file:drop" with the full native path.
    // The HTML5 DataTransfer API in WKWebView doesn't expose file.path.
    EventsOn("file:drop", (path: string) => {
      inputPath = path;
      dropError = "";
    });

    EventsOn("file:drop:error", (msg: string) => {
      dropError = msg;
    });

    // On Windows (WebView2), Go's OnFileDrop callback never fires unless a
    // JS-side OnFileDrop handler is also registered (Wails issue #3985).
    // Register a no-op handler here to activate the bridge.
    OnFileDrop((_x: number, _y: number, _paths: string[]) => {}, false);
  });

  onDestroy(() => {
    document.removeEventListener("dragover", blockNativeDrop);
    document.removeEventListener("drop", blockNativeDrop);
    EventsOff("file:drop");
    EventsOff("file:drop:error");
  });

  $: canProceed = inputPath !== "" && outputDir !== "";
</script>

<div class="wrapper">
  <div class="card">
    <p class="section-title">Исходный PDF</p>

    <!-- svelte-ignore a11y-no-static-element-interactions -->
    <div
      class="drop-zone"
      class:active={dragging}
      on:click={pickFile}
      on:dragover|preventDefault={handleDragOver}
      on:dragleave={handleDragLeave}
      on:drop|preventDefault={handleDrop}
    >
      {#if inputPath}
        <span class="drop-icon">📄</span>
        <span class="drop-name">{inputPath.split(/[\\/]/).pop()}</span>
        <span class="drop-hint">{inputPath}</span>
      {:else}
        <span class="drop-icon">⬆</span>
        <span class="drop-name">Перетащите PDF сюда</span>
        <span class="drop-hint">или нажмите чтобы выбрать</span>
      {/if}
    </div>
    {#if dropError}
      <p class="drop-error">{dropError}</p>
    {/if}
  </div>

  <div class="card">
    <p class="section-title">Папка для результатов</p>
    <div class="row">
      <span class="field-path" class:filled={outputDir !== ""}>
        {outputDir || "Папка не выбрана"}
      </span>
      <button class="btn btn-ghost" on:click={pickDir}>Выбрать…</button>
    </div>
  </div>

  <div class="actions">
    <button class="btn btn-primary" disabled={!canProceed} on:click={onReady}>
      Начать обработку
    </button>
  </div>
</div>

<style>
  .wrapper {
    display: flex;
    flex-direction: column;
    gap: 16px;
    padding: 24px;
    max-width: 640px;
    margin: 0 auto;
    width: 100%;
  }

  .card {
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 16px;
  }

  .drop-zone {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 4px;
    padding: 28px 20px;
    border: 2px dashed var(--border);
    border-radius: var(--radius);
    cursor: pointer;
    transition: border-color 0.15s, background 0.15s;
    text-align: center;
  }

  .drop-zone:hover, .drop-zone.active {
    border-color: var(--accent);
    background: rgba(79,142,247,0.06);
  }

  .drop-error {
    font-size: 12px;
    color: var(--danger);
    margin-top: 6px;
    text-align: center;
  }

  .drop-icon { font-size: 28px; line-height: 1; }
  .drop-name { font-size: 14px; color: var(--text); margin-top: 4px; }
  .drop-hint { font-size: 12px; color: var(--text-muted); }

  .row {
    display: flex;
    gap: 10px;
    align-items: center;
  }

  .actions {
    display: flex;
    justify-content: flex-end;
  }
</style>
