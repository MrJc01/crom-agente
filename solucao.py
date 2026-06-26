def is_bored(S):
    # Split the string into sentences
    sentences = S.split('. ')
    sentences = sentences[:-1] if sentences[-1] in ['?', '!'] else sentences
    
    # Count the number of sentences that start with "I"
    boredoms = sum(1 for sentence in sentences if sentence.strip().startswith('I '))
    
    return boredoms
    
PYEOFA data e hora atual é: 2026-06-26 06:17:49.950901
