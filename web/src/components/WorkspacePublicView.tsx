import React, { useState } from 'react';
import { PublicWorkspaceData, PublicNoteItem, PublicSheetItem } from '../lib/types';
import { FileText, Table, Tag } from 'lucide-react';

interface Props {
  workspace: PublicWorkspaceData;
}

export const WorkspacePublicView: React.FC<Props> = ({ workspace }) => {
  const [activeTab, setActiveTab] = useState<'notes' | 'sheets'>('notes');
  const [selectedNote, setSelectedNote] = useState<PublicNoteItem | null>(() => {
    return workspace.notes && workspace.notes.length > 0 ? workspace.notes[0] : null;
  });
  const [selectedSheet, setSelectedSheet] = useState<PublicSheetItem | null>(() => {
    return workspace.sheets && workspace.sheets.length > 0 ? workspace.sheets[0] : null;
  });

  const notes = workspace.notes || [];
  const sheets = workspace.sheets || [];

  const parseSheetCells = (dataJSON: string) => {
    try {
      const parsed = JSON.parse(dataJSON || '{}');
      return parsed.cells || {};
    } catch {
      return {};
    }
  };

  const renderCellDisplay = (cells: Record<string, any>, col: number, row: number) => {
    let colLetters = '';
    let c = col + 1;
    while (c > 0) {
      const rem = (c - 1) % 26;
      colLetters = String.fromCharCode(65 + rem) + colLetters;
      c = Math.floor((c - 1) / 26);
    }
    const coord = `${colLetters}${row + 1}`;
    const cell = cells[coord];
    if (!cell) return '';
    return cell.v !== undefined && cell.v !== null ? String(cell.v) : cell.raw || '';
  };

  return (
    <section className="card workspace-public-card">
      <div className="card-header flex justify-between items-center mb-4">
        <div className="flex items-center gap-2">
          <FileText className="w-5 h-5 text-blue-500" />
          <h2 className="text-lg font-semibold text-zinc-900 dark:text-zinc-100">
            Public Workspace (Pocket Studio)
          </h2>
        </div>
        <div className="flex gap-1 bg-zinc-100 dark:bg-zinc-800 p-1 rounded-lg">
          <button
            onClick={() => setActiveTab('notes')}
            className={`px-3 py-1 text-xs font-medium rounded-md transition-colors ${
              activeTab === 'notes'
                ? 'bg-white dark:bg-zinc-700 text-blue-600 dark:text-blue-400 shadow-sm'
                : 'text-zinc-600 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-100'
            }`}
          >
            Notes ({notes.length})
          </button>
          <button
            onClick={() => setActiveTab('sheets')}
            className={`px-3 py-1 text-xs font-medium rounded-md transition-colors ${
              activeTab === 'sheets'
                ? 'bg-white dark:bg-zinc-700 text-blue-600 dark:text-blue-400 shadow-sm'
                : 'text-zinc-600 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-100'
            }`}
          >
            Sheets ({sheets.length})
          </button>
        </div>
      </div>

      {activeTab === 'notes' ? (
        notes.length === 0 ? (
          <p className="text-sm text-zinc-500 dark:text-zinc-400 py-6 text-center">
            No public notes shared yet.
          </p>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
            <div className="flex flex-col gap-2 max-h-80 overflow-y-auto">
              {notes.map((n) => (
                <button
                  key={n.id}
                  onClick={() => setSelectedNote(n)}
                  className={`text-left p-3 rounded-lg border transition-all ${
                    selectedNote?.id === n.id
                      ? 'border-blue-500 bg-blue-50 dark:bg-blue-950/30'
                      : 'border-zinc-200 dark:border-zinc-800 hover:bg-zinc-50 dark:hover:bg-zinc-800/50'
                  }`}
                >
                  <div className="font-medium text-sm text-zinc-900 dark:text-zinc-100 truncate">
                    {n.title || 'Untitled Note'}
                  </div>
                  <div className="text-xs text-zinc-500 dark:text-zinc-400 truncate mt-1">
                    {n.content}
                  </div>
                </button>
              ))}
            </div>

            <div className="md:col-span-2 p-4 bg-zinc-50 dark:bg-zinc-900 rounded-lg border border-zinc-200 dark:border-zinc-800">
              {selectedNote ? (
                <div>
                  <h3 className="font-semibold text-base text-zinc-900 dark:text-zinc-100 mb-2">
                    {selectedNote.title}
                  </h3>
                  {selectedNote.tags && selectedNote.tags.length > 0 && (
                    <div className="flex flex-wrap gap-1 mb-3">
                      {selectedNote.tags.map((t) => (
                        <span
                          key={t}
                          className="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full bg-blue-100 dark:bg-blue-900/40 text-blue-700 dark:text-blue-300"
                        >
                          <Tag className="w-3 h-3" />
                          #{t}
                        </span>
                      ))}
                    </div>
                  )}
                  <pre className="text-xs font-mono text-zinc-800 dark:text-zinc-200 whitespace-pre-wrap leading-relaxed">
                    {selectedNote.content}
                  </pre>
                </div>
              ) : (
                <p className="text-sm text-zinc-500 text-center py-8">Select a note to read</p>
              )}
            </div>
          </div>
        )
      ) : sheets.length === 0 ? (
        <p className="text-sm text-zinc-500 dark:text-zinc-400 py-6 text-center">
          No public sheets shared yet.
        </p>
      ) : (
        <div className="flex flex-col gap-4">
          <div className="flex gap-2 overflow-x-auto pb-1">
            {sheets.map((s) => (
              <button
                key={s.id}
                onClick={() => setSelectedSheet(s)}
                className={`px-3 py-1.5 rounded-md text-xs font-medium border whitespace-nowrap ${
                  selectedSheet?.id === s.id
                    ? 'border-blue-500 bg-blue-50 dark:bg-blue-950/30 text-blue-600 dark:text-blue-400'
                    : 'border-zinc-200 dark:border-zinc-800 text-zinc-600 dark:text-zinc-400'
                }`}
              >
                <Table className="w-3.5 h-3.5 inline mr-1" />
                {s.title || 'Untitled Sheet'}
              </button>
            ))}
          </div>

          {selectedSheet && (
            <div className="overflow-x-auto border border-zinc-200 dark:border-zinc-800 rounded-lg max-h-72">
              {(() => {
                const cells = parseSheetCells(selectedSheet.data_json);
                const cols = Math.min(10, selectedSheet.col_count || 10);
                const rows = Math.min(20, selectedSheet.row_count || 20);

                return (
                  <table className="w-full text-xs border-collapse">
                    <thead>
                      <tr className="bg-zinc-100 dark:bg-zinc-800 text-zinc-600 dark:text-zinc-400">
                        <th className="p-1 border border-zinc-200 dark:border-zinc-700 w-8 text-center">#</th>
                        {Array.from({ length: cols }).map((_, c) => {
                          let colLetters = '';
                          let colNum = c + 1;
                          while (colNum > 0) {
                            const rem = (colNum - 1) % 26;
                            colLetters = String.fromCharCode(65 + rem) + colLetters;
                            colNum = Math.floor((colNum - 1) / 26);
                          }
                          return (
                            <th key={c} className="p-1 border border-zinc-200 dark:border-zinc-700 min-w-[70px] text-center">
                              {colLetters}
                            </th>
                          );
                        })}
                      </tr>
                    </thead>
                    <tbody>
                      {Array.from({ length: rows }).map((_, r) => (
                        <tr key={r} className="hover:bg-zinc-50 dark:hover:bg-zinc-800/40">
                          <td className="p-1 border border-zinc-200 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-800/60 text-zinc-500 text-center font-mono">
                            {r + 1}
                          </td>
                          {Array.from({ length: cols }).map((_, c) => (
                            <td key={c} className="p-1 border border-zinc-200 dark:border-zinc-800 text-zinc-800 dark:text-zinc-200 truncate">
                              {renderCellDisplay(cells, c, r)}
                            </td>
                          ))}
                        </tr>
                      ))}
                    </tbody>
                  </table>
                );
              })()}
            </div>
          )}
        </div>
      )}
    </section>
  );
};
