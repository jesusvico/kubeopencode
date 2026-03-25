import React, { useEffect, useRef, useState, useCallback } from 'react';
import { useMutation } from '@tanstack/react-query';
import api from '../api/client';

interface HITLPanelProps {
  namespace: string;
  taskName: string;
  isRunning: boolean;
}

interface PermissionRequest {
  id: string;
  sessionID: string;
  permission: string;
  patterns: string[];
  metadata?: Record<string, unknown>;
}

interface QuestionOption {
  label: string;
  description: string;
}

interface Question {
  question: string;
  header: string;
  options: QuestionOption[];
  multiple?: boolean;
  custom?: boolean;
}

interface QuestionRequest {
  id: string;
  sessionID: string;
  questions: Question[];
}

interface AgentMessage {
  id: string;
  type: 'text' | 'permission' | 'question' | 'status' | 'system';
  content: string;
  timestamp: Date;
  permissionRequest?: PermissionRequest;
  questionRequest?: QuestionRequest;
  resolved?: boolean;
}

function HITLPanel({ namespace, taskName, isRunning }: HITLPanelProps) {
  const [messages, setMessages] = useState<AgentMessage[]>([]);
  const [sessionId, setSessionId] = useState<string>('');
  const [isConnected, setIsConnected] = useState(false);
  const [messageInput, setMessageInput] = useState('');
  const [selectedAnswers, setSelectedAnswers] = useState<Record<number, string[]>>({});
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const eventSourceRef = useRef<EventSource | null>(null);

  const addMessage = useCallback((msg: Omit<AgentMessage, 'id' | 'timestamp'>) => {
    setMessages(prev => [...prev, {
      ...msg,
      id: `${Date.now()}-${Math.random().toString(36).slice(2)}`,
      timestamp: new Date(),
    }]);
  }, []);

  // Connect to SSE events
  useEffect(() => {
    if (!isRunning) return;

    const url = api.getTaskEventsUrl(namespace, taskName);
    const eventSource = new EventSource(url);
    eventSourceRef.current = eventSource;

    eventSource.onopen = () => {
      setIsConnected(true);
      addMessage({ type: 'system', content: 'Connected to agent session' });
    };

    eventSource.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data);
        const props = data.properties || data;

        switch (data.type) {
          case 'permission.asked':
            setSessionId(prev => prev || props.sessionID);
            addMessage({
              type: 'permission',
              content: `Permission requested: ${props.permission} on ${(props.patterns || []).join(', ')}`,
              permissionRequest: props,
            });
            break;

          case 'question.asked':
            setSessionId(prev => prev || props.sessionID);
            addMessage({
              type: 'question',
              content: props.questions?.[0]?.question || 'Agent has a question',
              questionRequest: props,
            });
            break;

          case 'session.status':
            if (props.status?.type) {
              addMessage({ type: 'status', content: `Session: ${props.status.type}` });
            }
            if (props.sessionID) {
              setSessionId(prev => prev || props.sessionID);
            }
            break;

          case 'message.part.delta':
            if (props.delta) {
              setMessages(prev => {
                const last = prev[prev.length - 1];
                if (last && last.type === 'text' && !last.resolved) {
                  return [...prev.slice(0, -1), { ...last, content: last.content + props.delta }];
                }
                return [...prev, {
                  id: `${Date.now()}-${Math.random().toString(36).slice(2)}`,
                  type: 'text',
                  content: props.delta,
                  timestamp: new Date(),
                }];
              });
            }
            break;

          case 'permission.replied':
          case 'question.replied':
          case 'question.rejected':
            // Mark the corresponding request as resolved
            setMessages(prev => prev.map(m => {
              if (m.type === 'permission' && m.permissionRequest?.id === props.requestID) {
                return { ...m, resolved: true };
              }
              if (m.type === 'question' && m.questionRequest?.id === props.requestID) {
                return { ...m, resolved: true };
              }
              return m;
            }));
            break;

          case 'stream.closed':
            setIsConnected(false);
            addMessage({ type: 'system', content: 'Stream closed' });
            break;
        }
      } catch {
        // Ignore parse errors for SSE comments
      }
    };

    eventSource.onerror = () => {
      setIsConnected(false);
    };

    return () => {
      eventSource.close();
    };
  }, [namespace, taskName, isRunning, addMessage]);

  // Auto-scroll
  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages]);

  // Permission reply mutation
  const permissionMutation = useMutation({
    mutationFn: ({ permissionId, reply }: { permissionId: string; reply: 'once' | 'always' | 'reject' }) =>
      api.replyPermission(namespace, taskName, permissionId, reply),
  });

  // Question reply mutation
  const questionMutation = useMutation({
    mutationFn: ({ questionId, answers }: { questionId: string; answers: string[][] }) =>
      api.replyQuestion(namespace, taskName, questionId, answers),
  });

  // Question reject mutation
  const rejectMutation = useMutation({
    mutationFn: (questionId: string) =>
      api.rejectQuestion(namespace, taskName, questionId),
  });

  // Send message mutation
  const messageMutation = useMutation({
    mutationFn: (message: string) =>
      api.sendMessage(namespace, taskName, sessionId, message),
    onSuccess: () => {
      setMessageInput('');
      addMessage({ type: 'text', content: `You: ${messageInput}`, resolved: true });
    },
  });

  // Interrupt mutation
  const interruptMutation = useMutation({
    mutationFn: () => api.interruptTask(namespace, taskName, sessionId),
    onSuccess: () => {
      addMessage({ type: 'system', content: 'Interrupt sent' });
    },
  });

  const handlePermissionReply = (permissionId: string, reply: 'once' | 'always' | 'reject') => {
    permissionMutation.mutate({ permissionId, reply });
  };

  const handleQuestionReply = (questionId: string, questions: Question[]) => {
    const answers = questions.map((_, idx) => selectedAnswers[idx] || []);
    questionMutation.mutate({ questionId, answers });
    setSelectedAnswers({});
  };

  const handleSendMessage = (e: React.FormEvent) => {
    e.preventDefault();
    if (!messageInput.trim() || !sessionId) return;
    messageMutation.mutate(messageInput.trim());
  };

  const toggleAnswer = (questionIdx: number, label: string, multiple: boolean) => {
    setSelectedAnswers(prev => {
      const current = prev[questionIdx] || [];
      if (multiple) {
        return { ...prev, [questionIdx]: current.includes(label) ? current.filter(a => a !== label) : [...current, label] };
      }
      return { ...prev, [questionIdx]: [label] };
    });
  };

  return (
    <div className="bg-stone-950 rounded-xl overflow-hidden border border-stone-800">
      {/* Header */}
      <div className="px-4 py-2.5 bg-stone-900 flex items-center justify-between border-b border-stone-800">
        <div className="flex items-center space-x-2.5">
          <span className="text-xs font-display font-medium text-stone-400 uppercase tracking-wider">Interactive Session</span>
          <span className={`inline-block w-1.5 h-1.5 rounded-full ${isConnected ? 'bg-emerald-400' : 'bg-stone-600'}`} />
        </div>
        <div className="flex items-center space-x-2">
          {sessionId && (
            <span className="text-[11px] text-stone-600 font-mono">{sessionId.slice(0, 8)}...</span>
          )}
          {isConnected && sessionId && (
            <button
              onClick={() => interruptMutation.mutate()}
              disabled={interruptMutation.isPending}
              className="px-2 py-1 text-[11px] font-medium text-amber-400 bg-amber-950/50 border border-amber-800/50 rounded hover:bg-amber-900/50 transition-colors"
            >
              {interruptMutation.isPending ? 'Sending...' : 'Interrupt'}
            </button>
          )}
        </div>
      </div>

      {/* Messages */}
      <div className="p-4 h-96 overflow-y-auto space-y-3 sidebar-scroll">
        {messages.length === 0 ? (
          <p className="text-stone-600 text-xs">Waiting for agent events...</p>
        ) : (
          messages.map((msg) => (
            <div key={msg.id} className="text-xs">
              {msg.type === 'system' && (
                <p className="text-stone-600 italic">{msg.content}</p>
              )}

              {msg.type === 'status' && (
                <p className="text-stone-500">{msg.content}</p>
              )}

              {msg.type === 'text' && (
                <div className="text-stone-300 whitespace-pre-wrap font-mono">{msg.content}</div>
              )}

              {msg.type === 'permission' && msg.permissionRequest && (
                <div className={`rounded-lg p-3 border ${msg.resolved ? 'bg-stone-900/50 border-stone-800' : 'bg-amber-950/30 border-amber-800/50'}`}>
                  <p className="text-amber-300 font-medium mb-1">Permission Required</p>
                  <p className="text-stone-400 mb-2">
                    <span className="font-mono text-amber-200">{msg.permissionRequest.permission}</span>
                    {' on '}
                    <span className="font-mono text-stone-300">{msg.permissionRequest.patterns?.join(', ')}</span>
                  </p>
                  {!msg.resolved && (
                    <div className="flex space-x-2">
                      <button
                        onClick={() => handlePermissionReply(msg.permissionRequest!.id, 'once')}
                        disabled={permissionMutation.isPending}
                        className="px-3 py-1.5 text-[11px] font-medium text-emerald-300 bg-emerald-950/50 border border-emerald-800/50 rounded hover:bg-emerald-900/50 transition-colors"
                      >
                        Allow Once
                      </button>
                      <button
                        onClick={() => handlePermissionReply(msg.permissionRequest!.id, 'always')}
                        disabled={permissionMutation.isPending}
                        className="px-3 py-1.5 text-[11px] font-medium text-blue-300 bg-blue-950/50 border border-blue-800/50 rounded hover:bg-blue-900/50 transition-colors"
                      >
                        Always
                      </button>
                      <button
                        onClick={() => handlePermissionReply(msg.permissionRequest!.id, 'reject')}
                        disabled={permissionMutation.isPending}
                        className="px-3 py-1.5 text-[11px] font-medium text-red-300 bg-red-950/50 border border-red-800/50 rounded hover:bg-red-900/50 transition-colors"
                      >
                        Reject
                      </button>
                    </div>
                  )}
                  {msg.resolved && (
                    <p className="text-stone-600 text-[11px]">Resolved</p>
                  )}
                </div>
              )}

              {msg.type === 'question' && msg.questionRequest && (
                <div className={`rounded-lg p-3 border ${msg.resolved ? 'bg-stone-900/50 border-stone-800' : 'bg-blue-950/30 border-blue-800/50'}`}>
                  <p className="text-blue-300 font-medium mb-2">Agent Question</p>
                  {msg.questionRequest.questions.map((q, qIdx) => (
                    <div key={qIdx} className="mb-2">
                      <p className="text-stone-300 mb-1">{q.question}</p>
                      {!msg.resolved && (
                        <div className="space-y-1 ml-2">
                          {q.options.map((opt) => (
                            <label key={opt.label} className="flex items-center space-x-2 cursor-pointer group">
                              <input
                                type={q.multiple ? 'checkbox' : 'radio'}
                                name={`q-${msg.id}-${qIdx}`}
                                checked={(selectedAnswers[qIdx] || []).includes(opt.label)}
                                onChange={() => toggleAnswer(qIdx, opt.label, !!q.multiple)}
                                className="accent-blue-500"
                              />
                              <span className="text-stone-300 group-hover:text-stone-100">{opt.label}</span>
                              {opt.description && (
                                <span className="text-stone-600">- {opt.description}</span>
                              )}
                            </label>
                          ))}
                        </div>
                      )}
                    </div>
                  ))}
                  {!msg.resolved && (
                    <div className="flex space-x-2 mt-2">
                      <button
                        onClick={() => handleQuestionReply(msg.questionRequest!.id, msg.questionRequest!.questions)}
                        disabled={questionMutation.isPending}
                        className="px-3 py-1.5 text-[11px] font-medium text-blue-300 bg-blue-950/50 border border-blue-800/50 rounded hover:bg-blue-900/50 transition-colors"
                      >
                        Submit
                      </button>
                      <button
                        onClick={() => rejectMutation.mutate(msg.questionRequest!.id)}
                        disabled={rejectMutation.isPending}
                        className="px-3 py-1.5 text-[11px] font-medium text-stone-400 bg-stone-900/50 border border-stone-700 rounded hover:bg-stone-800/50 transition-colors"
                      >
                        Skip
                      </button>
                    </div>
                  )}
                  {msg.resolved && (
                    <p className="text-stone-600 text-[11px]">Resolved</p>
                  )}
                </div>
              )}
            </div>
          ))
        )}
        <div ref={messagesEndRef} />
      </div>

      {/* Message input */}
      {isConnected && sessionId && (
        <form onSubmit={handleSendMessage} className="px-4 py-3 bg-stone-900 border-t border-stone-800 flex space-x-2">
          <input
            type="text"
            value={messageInput}
            onChange={(e) => setMessageInput(e.target.value)}
            placeholder="Send a follow-up message..."
            className="flex-1 bg-stone-800 text-stone-200 text-xs rounded-lg px-3 py-2 border border-stone-700 focus:outline-none focus:border-primary-500 placeholder:text-stone-600"
            disabled={messageMutation.isPending}
          />
          <button
            type="submit"
            disabled={!messageInput.trim() || messageMutation.isPending}
            className="px-4 py-2 text-xs font-medium text-stone-200 bg-primary-600 rounded-lg hover:bg-primary-500 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            Send
          </button>
        </form>
      )}
    </div>
  );
}

export default HITLPanel;
