<script lang="ts">
  import { OpenResultsFolder } from "../../wailsjs/go/app/App.js";
  import type { ProcessingResult } from "../types.js";

  export let result: ProcessingResult;
  export let outputDir: string;
  export let onReset: () => void;

  async function openFolder() {
    await OpenResultsFolder(outputDir);
  }
</script>

<div class="wrapper">

  <!-- Summary row -->
  <div class="summary-row">
    <div class="stat">
      <span class="stat-val tag tag-ok">{result.OutputFiles?.length ?? 0}</span>
      <span class="stat-lbl">студентов обработано</span>
    </div>
    {#if result.Orphans?.length}
      <div class="stat">
        <span class="stat-val tag tag-orphan">{result.Orphans.length}</span>
        <span class="stat-lbl">нераспознанных страниц</span>
      </div>
    {/if}
    {#if result.Errors?.length}
      <div class="stat">
        <span class="stat-val tag tag-orphan">{result.Errors.length}</span>
        <span class="stat-lbl">ошибок</span>
      </div>
    {/if}
  </div>

  <!-- Results table -->
  {#if result.OutputFiles?.length}
    <div class="card">
      <p class="section-title">Созданные файлы</p>
      <table class="data-table">
        <thead>
          <tr>
            <th>ФИО</th>
            <th>Группа</th>
            <th>Стр.</th>
            <th>Файл</th>
          </tr>
        </thead>
        <tbody>
          {#each result.OutputFiles as f}
            <tr>
              <td>{f.Student.FullName}</td>
              <td><span class="tag tag-group">{f.Student.Group}</span></td>
              <td class="center">{f.Student.Pages?.length ?? 0}</td>
              <td class="filename">{f.FileName}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}

  <!-- Orphans -->
  {#if result.Orphans?.length}
    <div class="card">
      <p class="section-title">Нераспознанные страницы</p>
      <table class="data-table">
        <thead>
          <tr>
            <th>№ страницы</th>
            <th>Статус</th>
          </tr>
        </thead>
        <tbody>
          {#each result.Orphans as op}
            <tr>
              <td>{op.Page.Number}</td>
              <td><span class="tag tag-orphan">Не распознана</span></td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}

  <!-- Errors -->
  {#if result.Errors?.length}
    <div class="card">
      <p class="section-title" style="color: var(--danger)">Ошибки</p>
      <ul class="error-list">
        {#each result.Errors as err}
          <li>{err}</li>
        {/each}
      </ul>
    </div>
  {/if}

  <!-- Actions -->
  <div class="actions">
    <button class="btn btn-ghost" on:click={onReset}>Обработать новый файл</button>
    <button class="btn btn-primary" on:click={openFolder}>Открыть папку</button>
  </div>

</div>

<style>
  .wrapper {
    display: flex;
    flex-direction: column;
    gap: 14px;
    padding: 24px;
    max-width: 720px;
    margin: 0 auto;
    width: 100%;
  }

  .card {
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 16px;
    overflow: hidden;
  }

  .summary-row {
    display: flex;
    gap: 16px;
    flex-wrap: wrap;
  }

  .stat {
    display: flex;
    align-items: center;
    gap: 8px;
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 10px 16px;
  }

  .stat-val { font-size: 14px; }
  .stat-lbl { font-size: 13px; color: var(--text-muted); }

  .filename {
    font-family: "Courier New", monospace;
    font-size: 12px;
    color: var(--text-muted);
    max-width: 240px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .center { text-align: center; }

  .actions {
    display: flex;
    justify-content: flex-end;
    gap: 10px;
  }
</style>
