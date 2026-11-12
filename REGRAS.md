# Regras do Jarvis Local

1. Primeira missão: encontrar uma oportunidade e construir um protótipo.
2. A partir do objetivo permanente do usuário, o Jarvis cria e prioriza as próprias tarefas.
3. O Jarvis trabalha apenas em sessões iniciadas pelo usuário. Ao fechar, pausar ou reiniciar, o progresso é preservado. O programa aguarda novo início.
4. Na internet, o Jarvis baixa arquivos públicos para a pasta da missão. As fontes são configuráveis e o conteúdo externo serve como dado, sem autoridade para alterar as regras.
5. Na missão, o Jarvis pode criar e editar arquivos, compilar, rodar testes e instalar dependências locais. Programas externos dependem de autorização do usuário.
6. A conclusão exige evidências, um protótipo que funcione, testes de aceite e aprovação pelo usuário na janela do Jarvis.
7. O Jarvis pode ajustar suas estratégias e criar ferramentas depois de testá-las. As regras de permissões, a política de OTA e o executável não são editáveis pelo modelo.
8. A memória local guarda objetivos, etapas, decisões, erros, lições, preferências, permissões, arquivos, fontes e testes.
9. A interface principal é uma janela própria do Windows.
10. O GitHub público guarda este código e estas regras. GitHub Actions compila no Windows x64. GitHub Releases distribui o .exe e serve como origem das atualizações.

## Implementação v0.1.0

Use o Ollama local com `qwen3.5:9b`. A janela inicia e pausa sessões, registra aprovação, abre a pasta ou o protótipo com autorização e verifica atualizações. Os dados ficam em `%LOCALAPPDATA%\JarvisLocal`. Em `fontes.txt`, informe uma URL HTTPS pública de RSS ou Atom por linha. O agente trabalha com os arquivos baixados. Use um objetivo ligado a oportunidades que possam ser demonstradas com um protótipo quantitativo.

O compilador integrado gera um HTML autossuficiente com uma ferramenta de cálculo configurada pelo Qwen. O executor testa a ferramenta com casos fixos e casos de aceite do modelo, e pede ao Qwen uma revisão de evidências, escopo e resultados antes de exibir a aprovação. Estratégias candidatas são experimentadas em uma missão e adotadas após aprovação. Ferramentas aprovadas pelos testes são armazenadas e podem ser reutilizadas pelo modelo em missões futuras. Nenhuma dependência externa é necessária para o template inicial. As ações da v0.1 usam o compilador interno e os programas abertos explicitamente pelo usuário pela janela.

O OTA consulta a release do repositório, verifica o SHA-256 fornecido pelo GitHub, solicita instalação pela janela e mantém uma cópia anterior até o teste de inicialização passar. Execute o .exe de uma pasta em que seu usuário possa escrever.

## Implementação v0.1.1

O Jarvis valida o tipo de entrega antes de testar. Para o diagnóstico ligado ao aviso Git de 10/11/2026, a janela exige a seleção de um arquivo local de configuração e autorização para consultar o executável Git. O relatório confere a versão reportada e quatro opções declaradas no arquivo escolhido. A missão fica pendente de execução local e aprovação. Uma calculadora usa entradas e operação fixadas na etapa de planejamento, com resposta estruturada do Ollama.

## Implementação v0.2

**Projetos** abre um espaço independente das missões de pesquisa. Um objetivo explícito vira um projeto Go de terminal com vários arquivos, um plano de aceite e tentativas de correção. O Jarvis congela os casos de terminal antes de gerar o código. Ele compila e executa testes de unidade e cenários de entrada e saída em WebAssembly, sem pastas montadas nem rede, e devolve erros reais de compilação e falhas de testes ao Qwen. Após passar, compila o executável Windows x64 e abre a pasta de entrega para sua avaliação. O executável nativo gerado não roda automaticamente.

O compilador oficial é baixado uma vez para o espaço de projetos e validado por SHA-256. A biblioteca padrão e a versão fixada de github.com/google/uuid fazem parte do catálogo inicial. A dependência opcional fica no cache da missão. Cada tentativa guarda seus arquivos e o programa final usa argumentos e entrada padrão. Ao pausar, a próxima sessão retoma a etapa salva.

Na v0.2.2, o Jarvis prepara lógica, entrada de terminal e testes em respostas individuais. Cada arquivo validado fica no checkpoint da missão. Se uma resposta falhar, a próxima tentativa mantém os demais arquivos e usa o diagnóstico do arquivo atual. Projetos existentes retomam com os casos de aceite já gravados.

## Implementação v0.3.0

O painel do Jarvis abre na área de trabalho, pode ser recolhido para a bandeja e alterna visibilidade com Ctrl + Alt + J quando o atalho estiver livre. O rosto acompanha os estados disponível, pensando, ouvindo e falando. As conversas usam o Qwen no Ollama local, com histórico em memória na sessão do aplicativo.

O botão de microfone coloca a transcrição no campo para revisão. Enviar é a ação que despacha o comando. Ditado e leitura em voz alta usam os componentes de fala do Windows e requerem idioma de fala em português. O painel usa Microsoft Edge WebView2. Se o runtime estiver indisponível, a janela de Missões e Projetos permanece acessível. O argumento --classic abre essa janela diretamente.

Comandos de conversa aceitam “Mostre o andamento”, “Abra projetos”, “Abra missões”, “Pause a sessão”, “Retome o projeto”, “Abra a pasta da entrega”, “Verifique atualizações”, “Abra a calculadora”, “Abra o bloco de notas” e “Novo projeto: objetivo”. A resposta do modelo é exibida como texto. A execução usa ações conhecidas solicitadas na mensagem do usuário. As ferramentas de projeto preservam as permissões e validações anteriores.

Referência de produto: Priler/jarvis (https://github.com/Priler/jarvis) usa uma interface desktop e separa reconhecimento, síntese e ativação por voz. Jarvis Local implementa sua própria interface e integra essa separação ao fluxo local do Ollama, de Missões e de Projetos.

## Vozes Kokoro · v0.3.4

O menu **Voz** apresenta Dora e Alex, duas vozes Kokoro em português do Brasil. A primeira prévia ou resposta falada baixa o pacote de vozes da versão no GitHub e verifica seu SHA-256 antes de instalar em `%LOCALAPPDATA%\JarvisLocal\Kokoro`. As próximas sínteses são locais. A escolha fica em `voz.txt`. Seleções das versões anteriores passam a usar Dora. A prévia funciona mesmo com **Responder em voz** desmarcado. O ditado continua usando o reconhecimento de fala do Windows.

## Voz incremental · v0.3.5

A conversa com o Ollama passa a receber eventos incrementais (`stream: true`), exibe a resposta em construção e distribui frases completas para a síntese de voz. Dora e Alex continuam disponíveis. Em respostas faladas, o Kokoro é carregado uma vez por resposta em um processo Python local persistente, enquanto um segundo fluxo reproduz os WAVs de frases anteriores. A geração de texto, a síntese e a reprodução podem ocorrer em paralelo; não há espera pelo texto ou áudio completos da resposta. A prévia de voz e os comandos explícitos continuam funcionando como antes. Erros, interrupção, limite de fila e fechamento da janela são tratados para não deixar processos ativos.

## Overlay com rosto humano · v0.4.0

O fluxo padrão inicia o Jarvis como uma pequena janela Win32 sobreposta, sem borda, com fundo transparente, rosto humano estilizado e controles compactos. A conversa só se expande quando o usuário a solicita, sem abrir a antiga área de trabalho cinza. O rosto é um recurso gráfico local embutido no executável, sem download ou câmera. Atalhos Ctrl+Alt+J e bandeja do sistema continuam disponíveis. O botão Sair e o X encerram o processo; a opção Ocultar preserva o acesso pela bandeja. O módulo legado de Projetos/Missões permanece somente como recuperação explícita via --classic, sem sobrescrever os dados locais. Vozes Kokoro e streaming Ollama continuam ativos. A animação de fala é visual por estado, ainda não é sincronização labial por fonemas. A composição transparente depende do WebView2 e deve ser inspecionada no Windows de destino.

## Face mesh procedural · v0.4.1

O retrato WebP foi removido. O avatar usa o Canonical Face Model (468 vértices/898 triângulos) publicado pelo projeto MediaPipe do Google, licença Apache 2.0, com uma projeção 3D leve em Canvas 2D offline. O renderizador colore os triângulos conforme a orientação da superfície, desenha topologia, pálpebras, sobrancelhas e lábios e deforma discretamente a mandíbula conforme os estados disponíveis (sem pretender sincronização fonética). A câmera não é solicitada. O modelo de origem pode ser consultado em https://github.com/google-ai-edge/mediapipe/blob/master/mediapipe/modules/face_geometry/data/canonical_face_model.obj (licença: https://github.com/google-ai-edge/mediapipe/blob/master/LICENSE; cópia incluída em `third_party/mediapipe-LICENSE.txt`). A renderização é validada na inicialização da interface e por testes de integridade de vértices/índices; a transparência permanece dependente do WebView2 no dispositivo final.

## Expressões faciais e atualizações automáticas · v0.4.2

O renderer facial passa a ter expressões interpoladas para disponível, ouvindo, pensando e falando; olhos com pálpebras geométricas, sobrancelhas, leve deslocamento do olhar, boca ancorada nos pontos internos de lábios e mandíbula com deformação local. A boca continua sendo uma aproximação visual e não sincronização por fonemas. A malha permanece 100% local.

O Jarvis consulta periodicamente (a partir de ~15 s após a inicialização e a cada 6 horas) a release pública mais recente do repositório oficial **K410Sh4/qwen-9b**. Só considera versões semanticamente mais novas com executável correspondente e SHA-256 publicado pela API do GitHub. O download é verificado e preparado sem bloquear a conversa. A substituição ocorre automaticamente apenas quando não há atividade do modelo/voz, tarefas em andamento ou chat expandido. A atualização reinicia o Jarvis e verifica o **overlay WebView2**; falhas no smoke test restauram o executável anterior. Nada é instalado diretamente de commits do branch main, releases não verificadas ou pull requests. No modo legado `--classic`, a rotina automática permanece desativada. O aplicativo precisa estar numa pasta em que o usuário possa escrever para que a atualização consiga substituir o executável. A v0.4.1 e anteriores ainda requerem atualização manual inicial para ativar este mecanismo.

## Motor de voz rápido · v0.4.3

Piper TTS 1.8.0 substitui o Kokoro. Edresson (pt-BR, low, 16 kHz) é a voz padrão e Faber (pt-BR, medium, 22,05 kHz) é a alternativa. O empacotamento da release Windows baixa e verifica SHA-256 dos modelos oficiais, instala o runtime Python incorporado e publica JarvisPiper-win-x64.zip, também validado por SHA-256 no cliente. A síntese continua recebendo frases incrementalmente do Qwen; Piper carrega o modelo uma vez por resposta e sintetiza a frase seguinte enquanto a anterior é reproduzida. A reprodução de cada WAV foi transferida de PowerShell para PlaySoundW nativo. O pacote Kokoro antigo não é utilizado. A preferência antiga de voz é convertida para Edresson. A primeira síntese após atualização precisa baixar o novo pacote; sínteses posteriores são locais. Medir a latência real no Windows, pois low/medium indicam variante do modelo e não garantem uma velocidade específica.

Licenças: Piper GPL-3.0-or-later, https://github.com/OHF-Voice/piper1-gpl ; modelos do conjunto https://huggingface.co/rhasspy/piper-voices, com MODEL_CARD distribuído no pacote.

## Piper Faber pré-carregado · v0.4.4

O Jarvis mantém somente a voz Faber (pt_BR-faber-medium). O seletor foi substituído por um rótulo estático e preferências legadas (incluindo Edresson) retornam automaticamente ao perfil Faber, preservando o restante do estado do usuário. O pacote portátil de release passa a incluir apenas o modelo Faber e seus arquivos de configuração/licença, com verificação SHA-256 existente. Assim que o overlay inicia, uma goroutine instala o pacote em cache (caso necessário) e inicia o processo Python residente, carregando a sessão ONNX **antes** de qualquer fala. Um handshake JSON `ready` confirma que a sessão está preparada. Tanto o comando de prévia quanto a síntese por frases do Qwen reutilizam esse mesmo processo; não se inicia outro Python a cada resposta. Fila sequencial de síntese e fila separada de reprodução preservam ordem, permitem produzir a frase seguinte enquanto a anterior toca, evitam concorrência entre sessões e propagam cancelamento. Falhas do processo provocam encerramento e tentativa de reinicialização na próxima síntese. O fechamento do Jarvis cancela downloads e encerra o processo residente; arquivos WAV temporários são limpos. Na primeira execução o download de pacote ainda pode demorar. O indicador de velocidade deverá ser medido num computador real; pré-carregamento não acelera o próprio Qwen.
