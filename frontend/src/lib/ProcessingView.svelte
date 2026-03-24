<script lang="ts">
  import { CancelProcessing } from "../../wailsjs/go/app/App.js";
  import type { ProcessingProgress } from "../types.js";

  export let progress: ProcessingProgress | null = null;
  export let error: string | null = null;
  export let onCancel: () => void;

  const stageLabel: Record<string, string> = {
    splitting: "Разделение страниц",
    rendering: "Рендеринг изображений",
    ocr:       "Распознавание текста",
    grouping:  "Группировка студентов",
    saving:    "Сохранение файлов",
  };

  $: percent = progress && progress.Total > 0
    ? Math.round((progress.Current / progress.Total) * 100)
    : 0;

  $: stage = progress ? (stageLabel[progress.Stage] ?? progress.Stage) : "";

  async function cancel() {
    await CancelProcessing();
    onCancel();
  }
</script>

<div class="wrapper">
  <div class="card">
    {#if error}
      <div class="error-state">
        <span class="err-icon">✕</span>
        <p class="err-title">Ошибка обработки</p>
        <p class="err-msg">{error}</p>
      </div>
    {/if}

    {#if !error}
      <p class="section-title">{stage || "Подготовка…"}</p>

      <div class="progress-track" style="margin-bottom: 8px;">
        <div class="progress-fill" style="width: {percent}%"></div>
      </div>

      <div class="meta">
        <span class="desc">{progress?.Description ?? "Запуск…"}</span>
        <span class="pct">{percent}%</span>
      </div>

      {#if progress?.Total > 0}
        <p class="count">{progress.Current} / {progress.Total}</p>
      {/if}
    {/if}
  </div>

  <div class="actions">
    {#if error}
      <button class="btn btn-ghost" on:click={onCancel}>← Назад</button>
    {:else}
      <button class="btn btn-danger" on:click={cancel}>Отмена</button>
    {/if}
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
    padding: 20px;
  }

  .meta {
    display: flex;
    justify-content: space-between;
    font-size: 12px;
    color: var(--text-muted);
  }

  .count {
    font-size: 12px;
    color: var(--text-muted);
    margin-top: 4px;
    text-align: right;
  }

  .error-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 6px;
    padding: 8px 0;
    text-align: center;
  }

  .err-icon {
    font-size: 28px;
    color: var(--danger);
    line-height: 1;
  }

  .err-title {
    font-weight: 600;
    color: var(--danger);
  }

  .err-msg {
    font-size: 13px;
    color: var(--text-muted);
    max-width: 400px;
    word-break: break-all;
  }

  .actions {
    display: flex;
    justify-content: flex-end;
  }
</style>
